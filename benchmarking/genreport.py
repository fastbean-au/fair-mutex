"""Regenerate the README benchmark tables and graphs from Go benchmark output.

Usage (from the repository root):

    go test -bench 'Locks/.*/UnderReadAndWriteLoad_(TrivialWork|ModestWork)$' \
        -run '^$' ./benchmarking/ > bench.out
    python3 benchmarking/genreport.py [bench.out [assets [tables.md]]]

Defaults: reads bench.out, writes the five PNGs into assets/, and writes the
four markdown tables to tables.md for pasting into README.md.

Requires matplotlib (e.g. `python3 -m venv .venv && .venv/bin/pip install
matplotlib`, then run with `.venv/bin/python`).
"""

import re
import sys

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

LINE_RE = re.compile(
    r"^BenchmarkLocks/(?P<impl>sync\.RWMutex|fairmutex)/"
    r"(?P<dir>Write|Read)_UnderReadAndWriteLoad_(?P<work>TrivialWork|ModestWork)/"
    r"(?:Writers|Readers)=(?P<n>\d+)-\d+\s+(?P<iters>\d+)\s+(?P<nsop>\d+(?:\.\d+)?) ns/op"
)

# (direction, work) -> (table/graph title, x-axis label, output image name)
GROUPS = {
    ("Read", "TrivialWork"): ("Trivial Readers", "Readers", "trivial_readers_benchmark.png"),
    ("Read", "ModestWork"): ("Modest Readers", "Readers", "modest_readers_benchmark.png"),
    ("Write", "TrivialWork"): ("Trivial Writers", "Writers", "trivial_writers_benchmark.png"),
    ("Write", "ModestWork"): ("Modest Writers", "Writers", "modest_writers_benchmark.png"),
}

# title -> (sync.RWMutex line style, fairmutex line style) for the combined chart
COMBINED_STYLES = {
    "Trivial Readers": (dict(color="red", linestyle="-", marker="o"),
                        dict(color="blue", linestyle="--", marker="s")),
    "Modest Readers": (dict(color="darkred", linestyle=":", marker="o"),
                       dict(color="green", linestyle="-.", marker="s")),
    "Trivial Writers": (dict(color="brown", linestyle="-", marker="x"),
                        dict(color="navy", linestyle="--", marker="P")),
    "Modest Writers": (dict(color="maroon", linestyle=":", marker="x"),
                       dict(color="darkgreen", linestyle="--", marker="P")),
}


def parse(path):
    data = {}  # (title, impl) -> {n: (iters, nsop)}
    with open(path) as f:
        for line in f:
            m = LINE_RE.match(line.strip())
            if not m:
                continue
            title = GROUPS[(m["dir"], m["work"])][0]
            data.setdefault((title, m["impl"]), {})[int(m["n"])] = (
                int(m["iters"]),
                float(m["nsop"]),
            )
    return data


def emit_tables(data, out):
    for title, axis, _ in GROUPS.values():
        sync = data[(title, "sync.RWMutex")]
        fair = data[(title, "fairmutex")]
        out.write(f"#### {title}\n\n")
        out.write(f"| {axis} | sync.RWMutex (ns/op) | sync.RWMutex (iters) | fairmutex (ns/op) | fairmutex (iters) |\n")
        out.write("|---------|----------------------|----------------------|-------------------|-------------------|\n")
        for n in sorted(sync):
            si, sns = sync[n]
            fi, fns = fair[n]
            out.write(f"| {n} | {round(sns):,} | {si:,} | {round(fns):,} | {fi:,} |\n")
        out.write("\n")


def plot_individual(data, assets):
    for title, axis, fname in GROUPS.values():
        sync = data[(title, "sync.RWMutex")]
        fair = data[(title, "fairmutex")]
        ns = sorted(sync)
        fig, ax = plt.subplots(figsize=(10, 6))
        ax.plot(ns, [sync[n][1] for n in ns], color="firebrick", linestyle="-", marker="o", label="sync.RWMutex")
        ax.plot(ns, [fair[n][1] for n in ns], color="darkblue", linestyle="--", marker="s", label="fairmutex")
        ax.set_yscale("log")
        ax.set_title(f"Benchmark: {title}")
        ax.set_xlabel(axis)
        ax.set_ylabel("Time (ns/op) - Log Scale")
        ax.set_xticks(ns)
        ax.grid(True, which="both", linestyle="--", alpha=0.5)
        ax.legend()
        fig.tight_layout()
        fig.savefig(f"{assets}/{fname}", dpi=100)
        plt.close(fig)


def plot_combined(data, assets):
    fig, ax = plt.subplots(figsize=(14, 10))
    for title, axis, _ in GROUPS.values():
        sync_style, fair_style = COMBINED_STYLES[title]
        sync = data[(title, "sync.RWMutex")]
        fair = data[(title, "fairmutex")]
        ns = sorted(sync)
        ax.plot(ns, [sync[n][1] for n in ns], label=f"sync.RWMutex ({title})", **sync_style)
        ax.plot(ns, [fair[n][1] for n in ns], label=f"fairmutex ({title})", **fair_style)
    ax.set_yscale("log")
    ax.set_title("Combined Benchmark Results (Log Scale)")
    ax.set_xlabel("Readers / Writers")
    ax.set_ylabel("Time (ns/op) - Log Scale")
    ax.set_xticks(range(1, 11))
    ax.grid(True, which="both", linestyle="--", alpha=0.5)
    ax.legend(loc="upper left", bbox_to_anchor=(1.02, 1.0))
    fig.tight_layout()
    fig.savefig(f"{assets}/combined_benchmarks.png", dpi=100)
    plt.close(fig)


def main():
    args = sys.argv[1:]
    bench_out = args[0] if len(args) > 0 else "bench.out"
    assets = args[1] if len(args) > 1 else "assets"
    tables_md = args[2] if len(args) > 2 else "tables.md"

    data = parse(bench_out)

    expected = {(title, impl) for (title, _, _) in GROUPS.values() for impl in ("sync.RWMutex", "fairmutex")}
    missing = expected - set(data)
    if missing:
        sys.exit(f"missing benchmark series: {missing}")
    for key, series in data.items():
        if len(series) != 10:
            sys.exit(f"series {key} has {len(series)} points, expected 10")

    with open(tables_md, "w") as f:
        emit_tables(data, f)
    plot_individual(data, assets)
    plot_combined(data, assets)
    print(f"tables written to {tables_md}; graphs written to {assets}/")


if __name__ == "__main__":
    main()
