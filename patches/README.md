# Local patches of MyGo

The vendored MyGo carries local changes Zeus needs, which upstream does
not have yet. A `go mod vendor` (or `go mod tidy` followed by one) drops
them; restore them with:

```sh
M=github.com/egoist/mygo
for d in patches/mygo/*.diff; do patch -p1 -d vendor/$M < "$d"; done
go build ./...
```

The patches:

- `scroll.go`, `plugins_terminal_*.diff` expose the emulator's scroll
  state (`AtBottom`, `ScrollState`, `ScrollToRow`, `ScrollToBottom`,
  `Rows`, `RowText`), which the find bar searches with, and add copy on
  select with its `OnCopy` event, raw OSC 7 change notifications with
  `OnPwd`, `SetScrollback`, `SetCursor` and
  `SetCopyOnSelect`, which the settings apply to running sessions.
- `ui_*.diff` add `MiddleClicked` to elements, which closes a tab with a
  middle click, and the headless tester's `MiddleClick`, which its test
  uses.
- `plugins_terminal_box.go.diff` draws Powerline half-circle caps to the
  terminal cell bounds, avoiding font-dependent vertical overhang.

Check them against a new MyGo when upgrading: the APIs they wrap may
have moved, and upstream may have grown the same features.
