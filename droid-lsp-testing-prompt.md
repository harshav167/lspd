# droid-lsp end-to-end testing prompt

Use `droid-lsp`, not `droid`.

## Goal

Verify that:

1. edit-time IDE/LSP diagnostics surface immediately
2. read-time hook diagnostics surface after `Read`
3. the `droid-lsp` wrapper is injecting the local settings file and hook path correctly
4. nothing is auto-fixed

## Prompt to run in `droid-lsp`

Create a temporary Go file named `lsp_diag_smoke.go` in the current directory with intentionally broken code that produces exactly these classes of diagnostics:

1. one undefined identifier
2. one mismatched return type
3. one unused import

Use this exact content:

```go
package main

import (
    "fmt"
    "strings"
)

func broken() string {
    return 123
}

func main() {
    fmt.Println(missingName)
}
```

After writing it:

1. do **not** fix anything
2. read the file immediately
3. report every diagnostic surfaced after the write
4. report every diagnostic surfaced after the read
5. explicitly say whether the read produced a hook-based `<system-reminder>`
6. do not repair the file

## Expected result

The session should surface diagnostics equivalent to:

- unused import: `strings`
- wrong return type: `cannot use 123 ... as string`
- undefined identifier: `missingName`

If the edit-time path works but the read-time hook path does not, report that explicitly.

## Cleanup prompt

After the test, remove `lsp_diag_smoke.go` and confirm it is gone.
