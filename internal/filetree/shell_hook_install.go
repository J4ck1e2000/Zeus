package filetree

import (
	"fmt"
	"strings"

	"zeus/internal/shellintegration"
)

func remoteShellInstaller(info RemoteShellInfo, hook shellintegration.File, uninstall bool) (string, error) {
	if err := validateRemoteShellInfo(info); err != nil {
		return "", err
	}
	expectedShell, expectedHook := remoteShellNames(info.Shell)
	if expectedShell == "" || hook.Name != expectedHook || strings.Contains(string(hook.Content), "__ZEUS_SHELL_HOOK_BODY_END__") {
		return "", fmt.Errorf("shell hook does not match remote shell %q", info.Shell)
	}
	home, err := quotePOSIX(info.Home)
	if err != nil {
		return "", err
	}
	quotedSource, err := quotePOSIX(remoteHookSourceLine(info.Shell, expectedHook))
	if err != nil {
		return "", err
	}
	quotedLegacySource, err := quotePOSIX(remoteHookSourceLineLegacy(info.Shell, expectedHook))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if uninstall {
		b.WriteString("# ZEUS_SHELL_HOOK_UNINSTALL\n")
	} else {
		b.WriteString("# ZEUS_SHELL_HOOK_INSTALL\n")
	}
	b.WriteString("set -u\numask 077\n")
	fmt.Fprintf(&b, "expected_shell=%s\nexpected_home=%s\n", quotePOSIXUnchecked(expectedShell), home)
	b.WriteString("shell_name=${SHELL##*/}\n")
	b.WriteString("[ \"$shell_name\" = \"$expected_shell\" ] || { printf '%s\\n' 'remote login shell changed' >&2; exit 1; }\n")
	b.WriteString("[ \"${HOME:-}\" = \"$expected_home\" ] || { printf '%s\\n' 'remote HOME changed' >&2; exit 1; }\n")
	b.WriteString("hook_dir=\"$HOME/.config/zeus/shell-integration\"\n")
	fmt.Fprintf(&b, "hook_file=\"$hook_dir/%s\"\n", expectedHook)
	fmt.Fprintf(&b, "hook_name=%s\n", quotePOSIXUnchecked(expectedHook))
	fmt.Fprintf(&b, "hook_start=%s\nhook_end=%s\n", quotePOSIXUnchecked(shellHookFileStartMarker), quotePOSIXUnchecked(shellHookFileEndMarker))
	fmt.Fprintf(&b, "block_start=%s\nblock_end=%s\nsource_line=%s\nlegacy_source_line=%s\n", quotePOSIXUnchecked(shellHookStartMarker), quotePOSIXUnchecked(shellHookEndMarker), quotedSource, quotedLegacySource)
	fmt.Fprintf(&b, "config_count=%d\n", len(info.StartupFiles))
	for i, startup := range info.StartupFiles {
		quoted, err := quotePOSIX(startup)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "config_%d=%s\n", i+1, quoted)
	}
	b.WriteString("txn_dir=\nhook_dir_created=0\n")
	b.WriteString(remoteTransactionHelpers)
	if uninstall {
		b.WriteString("if [ -L \"$hook_dir\" ]; then printf 'shell integration directory is a symlink\\n' >&2; exit 1; fi\n")
		b.WriteString("if [ ! -d \"$hook_dir\" ]; then mkdir -p \"$hook_dir\" || { printf 'could not create uninstall transaction directory\\n' >&2; exit 1; }; hook_dir_created=1; fi\n")
		b.WriteString("txn_dir=$(mktemp -d \"$hook_dir/.txn.XXXXXX\") || { printf 'could not create uninstall transaction\\n' >&2; exit 1; }\n")
		b.WriteString("trap 'fail \"uninstall interrupted\"' HUP INT TERM\n")
		b.WriteString(remoteUninstallBody)
		for i := range info.StartupFiles {
			fmt.Fprintf(&b, "stage_config_remove %d \"$config_%d\" || fail 'could not prepare startup file removal'\n", i+1, i+1)
		}
		b.WriteString("stage_hook_remove || fail 'could not prepare hook removal'\n")
		b.WriteString("commit_uninstall || fail 'could not commit uninstall'\ntrap - HUP INT TERM\nrm -rf \"$txn_dir\"\nif [ \"$hook_dir_created\" = 1 ]; then rmdir \"$hook_dir\" 2>/dev/null || :; fi\n")
	} else {
		b.WriteString("if [ -L \"$hook_dir\" ]; then printf 'shell integration directory is a symlink\\n' >&2; exit 1; fi\n")
		b.WriteString("if [ ! -d \"$hook_dir\" ]; then mkdir -p \"$hook_dir\" || { printf 'could not create shell integration directory\\n' >&2; exit 1; }; hook_dir_created=1; fi\n")
		b.WriteString("txn_dir=$(mktemp -d \"$hook_dir/.txn.XXXXXX\") || { printf 'could not create installation transaction\\n' >&2; exit 1; }\n")
		b.WriteString("trap 'fail \"install interrupted\"' HUP INT TERM\n")
		appendRemoteHookStage(&b, hook.Content)
		b.WriteString(remoteInstallCommit)
		for i, startup := range info.StartupFiles {
			quoted, err := quotePOSIX(startup)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "config_%d=%s\n", i+1, quoted)
			fmt.Fprintf(&b, "stage_config %d \"$config_%d\" || fail 'could not prepare startup file'\n", i+1, i+1)
		}
		b.WriteString("stage_hook || fail 'could not prepare the shell hook'\n")
		b.WriteString("commit_install || fail 'could not commit shell integration'\ntrap - HUP INT TERM\nrm -rf \"$txn_dir\"\n")
	}
	return b.String(), nil
}

func appendRemoteHookStage(b *strings.Builder, content []byte) {
	b.WriteString(`stage_hook() {
  if [ -L "$hook_file" ]; then return 1; fi
  if [ -e "$hook_file" ]; then
    [ -f "$hook_file" ] || return 1
    hook_start_count=$(grep -Fxc "$hook_start" "$hook_file" 2>/dev/null || :)
    hook_end_count=$(grep -Fxc "$hook_end" "$hook_file" 2>/dev/null || :)
    [ "$hook_start_count" = 1 ] && [ "$hook_end_count" = 1 ] || return 1
    valid_managed_block "$hook_file" "$hook_start" "$hook_end" "" "" || return 1
    cp -p "$hook_file" "$txn_dir/hook.original" || return 1
    printf '1\n' > "$txn_dir/hook.existed"
    hook_backup="$hook_file.zeus-shell-integration.bak"
    if [ -e "$hook_backup" ] && [ ! -f "$hook_backup" ]; then return 1; fi
    if [ ! -e "$hook_backup" ]; then
      cp -p "$hook_file" "$hook_backup" || return 1
      printf '%s\n' "$hook_backup" > "$txn_dir/hook.backup"
      : > "$txn_dir/hook.backup_created"
    fi
  else
    : > "$txn_dir/hook.original"
    printf '0\n' > "$txn_dir/hook.existed"
  fi
  hook_tmp=$(mktemp "$hook_dir/.zeus-hook.XXXXXX") || return 1
  printf '%s\n' "$hook_tmp" > "$txn_dir/hook.temp"
  {
    printf '%s\n' "$hook_start"
    cat <<'__ZEUS_SHELL_HOOK_BODY_END__'
`)
	b.Write(content)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(`__ZEUS_SHELL_HOOK_BODY_END__
    printf '%s\n' "$hook_end"
  } > "$hook_tmp" || return 1
  chmod 600 "$hook_tmp" || return 1
  return 0
}
`)
}

func remoteShellNames(shell shellintegration.Shell) (string, string) {
	switch shell {
	case shellintegration.Bash:
		return "bash", "bash-hook.sh"
	case shellintegration.Zsh:
		return "zsh", "zsh-hook.zsh"
	case shellintegration.Fish:
		return "fish", "fish-hook.fish"
	default:
		return "", ""
	}
}

func remoteHookSourceLine(shell shellintegration.Shell, hookName string) string {
	path := `"$HOME/.config/zeus/shell-integration/` + hookName + `"`
	switch shell {
	case shellintegration.Bash:
		return `[ -n "${BASH_VERSION:-}" ] && [ -r ` + path + ` ] && . ` + path
	case shellintegration.Fish:
		return "source " + path
	default:
		return "[ -r " + path + " ] && . " + path
	}
}

func remoteHookSourceLineLegacy(shell shellintegration.Shell, hookName string) string {
	if shell != shellintegration.Bash {
		return remoteHookSourceLine(shell, hookName)
	}
	path := `"$HOME/.config/zeus/shell-integration/` + hookName + `"`
	return "[ -r " + path + " ] && . " + path
}

func quotePOSIX(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("remote path contains a control character")
	}
	return quotePOSIXUnchecked(value), nil
}

func quotePOSIXUnchecked(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

const remoteTransactionHelpers = `
resolve_file() {
  resolve_target=$1
  resolve_hops=0
  while [ -L "$resolve_target" ]; do
    resolve_hops=$((resolve_hops + 1))
    [ "$resolve_hops" -le 32 ] || return 1
    resolve_link=$(readlink "$resolve_target") || return 1
    case "$resolve_link" in
      /*) resolve_target=$resolve_link ;;
      *) resolve_target="$(dirname "$resolve_target")/$resolve_link" ;;
    esac
  done
  printf '%s\n' "$resolve_target"
}

valid_managed_block() {
  validation_path=$1
  validation_start=$2
  validation_end=$3
  validation_source=$4
  validation_legacy_source=$5
  awk -v start="$validation_start" -v end="$validation_end" -v source="$validation_source" -v legacy="$validation_legacy_source" '
    BEGIN { inside=0; bad=0; starts=0; ends=0; sources=0 }
    $0 == start { starts++; if (inside || starts > 1) bad=1; inside=1; sources=0; next }
    $0 == end { ends++; if (!inside || ends > 1) bad=1; if (source != "" && sources != 1) bad=1; inside=0; next }
    inside && ((source != "" && $0 == source) || (legacy != "" && $0 == legacy)) { sources++ }
    END { if (inside || bad || starts != 1 || ends != 1) exit 1 }
  ' "$validation_path"
}

rollback() {
  trap - HUP INT TERM
  if [ -n "$txn_dir" ] && [ -d "$txn_dir" ]; then
    if [ -f "$txn_dir/hook.removed" ]; then
      mv "$txn_dir/hook.removed" "$hook_file" 2>/dev/null || :
    elif [ -f "$txn_dir/hook.committing" ]; then
      if [ "$(cat "$txn_dir/hook.existed")" = 1 ]; then
        cp -p "$txn_dir/hook.original" "$hook_file" 2>/dev/null || :
      else
        rm -f "$hook_file"
      fi
    fi
    rollback_index=1
    while [ "$rollback_index" -le "$config_count" ]; do
      if [ -f "$txn_dir/$rollback_index.removed" ]; then
        rollback_path=$(cat "$txn_dir/$rollback_index.path")
        mv "$txn_dir/$rollback_index.removed" "$rollback_path" 2>/dev/null || :
      elif [ -f "$txn_dir/$rollback_index.committing" ]; then
        rollback_path=$(cat "$txn_dir/$rollback_index.path")
        rollback_existed=$(cat "$txn_dir/$rollback_index.existed")
        if [ "$rollback_existed" = 1 ]; then
          cp -p "$txn_dir/$rollback_index.original" "$rollback_path" 2>/dev/null || :
        else
          rm -f "$rollback_path"
        fi
      fi
      if [ -f "$txn_dir/$rollback_index.temp" ]; then
        rm -f "$(cat "$txn_dir/$rollback_index.temp")"
      fi
      if [ -f "$txn_dir/$rollback_index.backup_created" ]; then
        rm -f "$(cat "$txn_dir/$rollback_index.backup")"
      fi
      rollback_index=$((rollback_index + 1))
    done
    if [ -f "$txn_dir/hook.temp" ]; then
      rm -f "$(cat "$txn_dir/hook.temp")"
    fi
    if [ -f "$txn_dir/hook.backup_created" ]; then
      rm -f "$(cat "$txn_dir/hook.backup")"
    fi
    rm -rf "$txn_dir"
  fi
  if [ "$hook_dir_created" = 1 ]; then rmdir "$hook_dir" 2>/dev/null || :; fi
}

fail() {
  printf '%s\n' "$1" >&2
  rollback
  exit 1
}

stage_config() {
  stage_index=$1
  stage_requested=$2
  stage_path=$(resolve_file "$stage_requested") || return 1
  stage_dir=$(dirname "$stage_path")
  [ -d "$stage_dir" ] || mkdir -p "$stage_dir" || return 1
  if [ -e "$stage_path" ] && [ ! -f "$stage_path" ]; then return 1; fi
  if [ -f "$stage_path" ] && { [ ! -r "$stage_path" ] || [ ! -w "$stage_path" ]; }; then return 1; fi
  printf '%s\n' "$stage_path" > "$txn_dir/$stage_index.path"
  if [ -f "$stage_path" ]; then
    cp -p "$stage_path" "$txn_dir/$stage_index.original" || return 1
    printf '1\n' > "$txn_dir/$stage_index.existed"
    stage_start=$(grep -Fxc "$block_start" "$stage_path" 2>/dev/null || :)
    stage_end=$(grep -Fxc "$block_end" "$stage_path" 2>/dev/null || :)
    if [ "$stage_start" = 1 ] && [ "$stage_end" = 1 ]; then
      valid_managed_block "$stage_path" "$block_start" "$block_end" "$source_line" "$legacy_source_line" || return 1
      printf '0\n' > "$txn_dir/$stage_index.changed"
      return 0
    fi
    if [ "$stage_start" != 0 ] || [ "$stage_end" != 0 ]; then return 1; fi
    stage_backup="$stage_path.zeus-shell-integration.bak"
    if [ -e "$stage_backup" ] && [ ! -f "$stage_backup" ]; then return 1; fi
    if [ ! -e "$stage_backup" ]; then
      cp -p "$stage_path" "$stage_backup" || return 1
      printf '%s\n' "$stage_backup" > "$txn_dir/$stage_index.backup"
      : > "$txn_dir/$stage_index.backup_created"
    fi
  else
    : > "$txn_dir/$stage_index.original"
    printf '0\n' > "$txn_dir/$stage_index.existed"
  fi
  stage_tmp=$(mktemp "$stage_dir/.zeus-shell-integration.XXXXXX") || return 1
  printf '%s\n' "$stage_tmp" > "$txn_dir/$stage_index.temp"
  if [ -f "$stage_path" ]; then cp -p "$stage_path" "$stage_tmp" || return 1; fi
  if [ -s "$stage_tmp" ]; then
    stage_last=$(tail -c 1 "$stage_tmp" 2>/dev/null || :)
    [ -z "$stage_last" ] || printf '\n' >> "$stage_tmp"
  fi
  printf '%s\n%s\n%s\n' "$block_start" "$source_line" "$block_end" >> "$stage_tmp" || return 1
  printf '1\n' > "$txn_dir/$stage_index.changed"
}
`

const remoteInstallCommit = `
commit_install() {
  hook_tmp=$(cat "$txn_dir/hook.temp") || return 1
  : > "$txn_dir/hook.committing" || return 1
  mv "$hook_tmp" "$hook_file" || return 1
  commit_index=1
  while [ "$commit_index" -le "$config_count" ]; do
    if [ -f "$txn_dir/$commit_index.changed" ] && [ "$(cat "$txn_dir/$commit_index.changed")" = 1 ]; then
      commit_path=$(cat "$txn_dir/$commit_index.path") || return 1
      commit_tmp=$(cat "$txn_dir/$commit_index.temp") || return 1
      : > "$txn_dir/$commit_index.committing" || return 1
      mv "$commit_tmp" "$commit_path" || return 1
    fi
    commit_index=$((commit_index + 1))
  done
  return 0
}
`

const remoteUninstallBody = `
stage_config_remove() {
  remove_index=$1
  remove_requested=$2
  remove_path=$(resolve_file "$remove_requested") || return 1
  printf '%s\n' "$remove_path" > "$txn_dir/$remove_index.path"
  if [ ! -e "$remove_path" ]; then
    printf '0\n' > "$txn_dir/$remove_index.changed"
    printf '0\n' > "$txn_dir/$remove_index.existed"
    return 0
  fi
  [ -f "$remove_path" ] || return 1
  cp -p "$remove_path" "$txn_dir/$remove_index.original" || return 1
  printf '1\n' > "$txn_dir/$remove_index.existed"
  remove_start=$(grep -Fxc "$block_start" "$remove_path" 2>/dev/null || :)
  remove_end=$(grep -Fxc "$block_end" "$remove_path" 2>/dev/null || :)
  if [ "$remove_start" = 0 ] && [ "$remove_end" = 0 ]; then
    printf '0\n' > "$txn_dir/$remove_index.changed"
    return 0
  fi
  if [ "$remove_start" != 1 ] || [ "$remove_end" != 1 ]; then return 1; fi
  valid_managed_block "$remove_path" "$block_start" "$block_end" "$source_line" "$legacy_source_line" || return 1
  remove_dir=$(dirname "$remove_path")
  remove_tmp=$(mktemp "$remove_dir/.zeus-shell-integration.XXXXXX") || return 1
  printf '%s\n' "$remove_tmp" > "$txn_dir/$remove_index.temp"
  awk -v start="$block_start" -v end="$block_end" '
    BEGIN { inside=0; bad=0; starts=0; ends=0 }
    $0 == start { starts++; if (inside || starts > 1) bad=1; inside=1; next }
    $0 == end { ends++; if (!inside || ends > 1) bad=1; inside=0; next }
    !inside { print }
    END { if (inside || bad || starts != 1 || ends != 1) exit 2 }
  ' "$remove_path" > "$remove_tmp" || return 1
  if [ ! -s "$remove_tmp" ] && [ ! -e "$remove_path.zeus-shell-integration.bak" ]; then
    : > "$txn_dir/$remove_index.remove_file"
  fi
  printf '1\n' > "$txn_dir/$remove_index.changed"
}

stage_hook_remove() {
  if [ ! -e "$hook_file" ]; then
    : > "$txn_dir/hook.existed"
    return 0
  fi
  [ -f "$hook_file" ] || return 1
  hook_start_count=$(grep -Fxc "$hook_start" "$hook_file" 2>/dev/null || :)
  hook_end_count=$(grep -Fxc "$hook_end" "$hook_file" 2>/dev/null || :)
  [ "$hook_start_count" = 1 ] && [ "$hook_end_count" = 1 ] || return 1
  valid_managed_block "$hook_file" "$hook_start" "$hook_end" "" "" || return 1
  cp -p "$hook_file" "$txn_dir/hook.original" || return 1
  printf '1\n' > "$txn_dir/hook.existed"
  : > "$txn_dir/hook.changed"
}

commit_uninstall() {
  commit_index=1
  while [ "$commit_index" -le "$config_count" ]; do
    if [ -f "$txn_dir/$commit_index.changed" ] && [ "$(cat "$txn_dir/$commit_index.changed")" = 1 ]; then
      commit_path=$(cat "$txn_dir/$commit_index.path") || return 1
      commit_tmp=$(cat "$txn_dir/$commit_index.temp") || return 1
      if [ -f "$txn_dir/$commit_index.remove_file" ]; then
        mv "$commit_path" "$txn_dir/$commit_index.removed" || return 1
      else
        : > "$txn_dir/$commit_index.committing" || return 1
        mv "$commit_tmp" "$commit_path" || return 1
      fi
    fi
    commit_index=$((commit_index + 1))
  done
  if [ -f "$txn_dir/hook.changed" ]; then
    mv "$hook_file" "$txn_dir/hook.removed" || return 1
  fi
  return 0
}
`
