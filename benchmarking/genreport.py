"""Regenerate the README benchmark tables and graphs from Go benchmark output.

Usage (from the repository root):

    go test -bench . -run '^$' ./benchmarking/ > bench.out
    python3 benchmarking/genreport.py [bench.out [assets [tables.md]]]

Defaults: reads bench.out, writes the PNGs into assets/, and writes the
markdown tables to tables.md for pasting into README.md.

Requires matplotlib (e.g. `python3 -m venv .venv && .venv/bin/pip install
matplotlib`, then run with `.venv/bin/python`).
"""

import re
import sys

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

IMPLS = ("sync.RWMutex", "fairmutex")

LINE_RE = re.compile(
    r"^BenchmarkLocks/(?P<impl>sync\.RWMutex|fairmutex)/(?P<rest>\S+?)-\d+\s+"
    r"(?P<iters>\d+)\s+(?P<metrics>.*)$"
)

METRIC_RE = re.compile(r"([\d.]+) ([\w/-]+)")


def parse(path):
    """Returns {(impl, benchmark-path): {metric-name: value}}."""
    data = {}
    with open(path) as f:
        for line in f:
            m = LINE_RE.match(line.strip())
            if not m:
                continue
            metrics = {name: float(v) for v, name in METRIC_RE.findall(m["metrics"])}
            data[(m["impl"], m["rest"])] = metrics
    return data


def ns(data, impl, rest, metric="ns/op"):
    return data[(impl, rest)][metric]


def fmt(v):
    return f"{round(v):,}"


def emit_tables(data, out):
    out.write("#### Uncontended\n\n")
    out.write("| Operation | sync.RWMutex (ns/op) | fairmutex (ns/op) |\n")
    out.write("|-----------|----------------------|-------------------|\n")
    for op in ("Lock", "RLock"):
        s = ns(data, "sync.RWMutex", f"Uncontended/{op}")
        f_ = ns(data, "fairmutex", f"Uncontended/{op}")
        out.write(f"| {op} | {fmt(s)} | {fmt(f_)} |\n")
    out.write("\n")

    out.write("#### Saturated read throughput\n\n")
    out.write("| | sync.RWMutex (ns/read) | fairmutex (ns/read) |\n")
    out.write("|-|------------------------|---------------------|\n")
    s = ns(data, "sync.RWMutex", "SaturatedReadThroughput")
    f_ = ns(data, "fairmutex", "SaturatedReadThroughput")
    out.write(f"| All processors reading | {fmt(s)} | {fmt(f_)} |\n")
    out.write("\n")

    for family, axis, title in (
        ("WriteLatency_SaturatedReads", "Writers", "Write lock latency under saturated reads"),
        ("ReadLatency_ContendingWrites", "Readers", "Read lock latency under contending writes"),
    ):
        out.write(f"#### {title}\n\n")
        out.write(f"| {axis} | sync.RWMutex mean (ns) | sync.RWMutex p99 (ns) | fairmutex mean (ns) | fairmutex p99 (ns) |\n")
        out.write("|---------|------------------------|-----------------------|---------------------|--------------------|\n")
        for n in range(1, 11):
            rest = f"{family}/{axis}={n}"
            row = [fmt(ns(data, impl, rest, metric)) for impl in IMPLS for metric in ("ns/op", "p99-ns")]
            out.write(f"| {n} | {row[0]} | {row[1]} | {row[2]} | {row[3]} |\n")
        out.write("\n")


def plot_latency(data, assets, family, axis, title, fname):
    xs = list(range(1, 11))
    fig, ax = plt.subplots(figsize=(10, 6))

    styles = {
        ("sync.RWMutex", "ns/op"): dict(color="firebrick", linestyle="-", marker="o", label="sync.RWMutex mean"),
        ("sync.RWMutex", "p99-ns"): dict(color="firebrick", linestyle=":", marker="o", label="sync.RWMutex p99"),
        ("fairmutex", "ns/op"): dict(color="darkblue", linestyle="-", marker="s", label="fairmutex mean"),
        ("fairmutex", "p99-ns"): dict(color="darkblue", linestyle=":", marker="s", label="fairmutex p99"),
    }
    for (impl, metric), style in styles.items():
        ys = [ns(data, impl, f"{family}/{axis}={n}", metric) for n in xs]
        ax.plot(xs, ys, **style)

    ax.set_yscale("log")
    ax.set_title(title)
    ax.set_xlabel(axis)
    ax.set_ylabel("Latency (ns) - Log Scale")
    ax.set_xticks(xs)
    ax.grid(True, which="both", linestyle="--", alpha=0.5)
    ax.legend()
    fig.tight_layout()
    fig.savefig(f"{assets}/{fname}", dpi=100)
    plt.close(fig)


def plot_combined(data, assets):
    xs = list(range(1, 11))
    fig, ax = plt.subplots(figsize=(12, 8))

    series = (
        ("sync.RWMutex", "WriteLatency_SaturatedReads", "Writers",
         dict(color="firebrick", linestyle="-", marker="o", label="sync.RWMutex write latency")),
        ("fairmutex", "WriteLatency_SaturatedReads", "Writers",
         dict(color="darkblue", linestyle="-", marker="s", label="fairmutex write latency")),
        ("sync.RWMutex", "ReadLatency_ContendingWrites", "Readers",
         dict(color="firebrick", linestyle="--", marker="x", label="sync.RWMutex read latency")),
        ("fairmutex", "ReadLatency_ContendingWrites", "Readers",
         dict(color="darkblue", linestyle="--", marker="P", label="fairmutex read latency")),
    )
    for impl, family, axis, style in series:
        ys = [ns(data, impl, f"{family}/{axis}={n}") for n in xs]
        ax.plot(xs, ys, **style)

    ax.set_yscale("log")
    ax.set_title("Mean lock latency under contention (Log Scale)")
    ax.set_xlabel("Contending writers / readers")
    ax.set_ylabel("Latency (ns) - Log Scale")
    ax.set_xticks(xs)
    ax.grid(True, which="both", linestyle="--", alpha=0.5)
    ax.legend()
    fig.tight_layout()
    fig.savefig(f"{assets}/combined_benchmarks.png", dpi=100)
    plt.close(fig)


def main():
    args = sys.argv[1:]
    bench_out = args[0] if len(args) > 0 else "bench.out"
    assets = args[1] if len(args) > 1 else "assets"
    tables_md = args[2] if len(args) > 2 else "tables.md"

    data = parse(bench_out)

    expected = ["Uncontended/Lock", "Uncontended/RLock", "SaturatedReadThroughput"]
    expected += [f"WriteLatency_SaturatedReads/Writers={n}" for n in range(1, 11)]
    expected += [f"ReadLatency_ContendingWrites/Readers={n}" for n in range(1, 11)]
    missing = {(impl, rest) for impl in IMPLS for rest in expected} - set(data)
    if missing:
        sys.exit(f"missing benchmark results: {sorted(missing)}")

    with open(tables_md, "w") as f:
        emit_tables(data, f)

    plot_latency(data, assets, "WriteLatency_SaturatedReads", "Writers",
                 "Write lock latency under 100 saturating readers", "write_latency_benchmark.png")
    plot_latency(data, assets, "ReadLatency_ContendingWrites", "Readers",
                 "Read lock latency under 4 contending writers", "read_latency_benchmark.png")
    plot_combined(data, assets)
    print(f"tables written to {tables_md}; graphs written to {assets}/")


if __name__ == "__main__":
    main()
