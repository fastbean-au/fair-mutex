# FairMutexCheck

Static code analysis for fair-mutex

## Usage Instructions

### Build
```bash
go build -o fairmutexcheck ./cmd/fairmutexcheck
```

### Run the checker
```bash
./fairmutexcheck ./...
./fairmutexcheck path/to/package
```

### Integrate with go vet
```bash
go vet -vettool=./fairmutexcheck ./...
```

## What this checker catches

Direct instantiation (use `fairmutex.New()` instead):

- ✗ `x := fairmutex.RWMutex{}`
- ✗ `x := &fairmutex.RWMutex{}`
- ✗ `x := new(fairmutex.RWMutex)`

Missing `Stop()` call (all of the following are accepted):

- ✓ `x := fairmutex.New(); defer x.Stop()`
- ✓ `x := fairmutex.New(); x.Stop()`
- ✓ `x := fairmutex.New(); go func() { x.Stop() }()`
- ✓ `x := fairmutex.New(); go func() { defer x.Stop() }()`

## Limitations

- If a mutex is returned from a function, the checker will warn — `Stop()` responsibility is assumed to move to the caller, which cannot be verified.
- `Stop()` passed as a parameter to a helper function is not detected (e.g. `helper(x)` where `helper` calls `x.Stop()`).
