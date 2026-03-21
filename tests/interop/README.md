# Interop Tests

This package contains bidirectional compatibility tests between `go-git` and
the Git CLI.

The suite is intentionally aimed at modern Git releases. It currently assumes
Git 2.28 or newer because the test helpers initialize repositories with:

```sh
git init -b main
```

That is deliberate. These tests are meant to validate current interoperability,
not preserve coverage for older Git versions with different defaults and setup
behavior.

Run the suite with:

```sh
go test -tags interop -v ./tests/interop/...
```

Or via the Make target:

```sh
make test-interop
```
