function __zeus_encode_path {
  LC_ALL=C printf '%s' "$1" |
    LC_ALL=C od -An -v -tx1 |
    LC_ALL=C awk '{ for (i = 1; i <= NF; i++) { b = toupper($i); if (b == "2F") printf "/"; else printf "%%%s", b } }'
}

function __zeus_report_dir {
  local cwd encoded host
  cwd=$PWD
  [[ $cwd == /* ]] || return
  encoded=$(__zeus_encode_path "$cwd") || return
  host=${HOSTNAME:-${HOST:-}}
  [[ -n $host ]] || host=$(hostname 2>/dev/null) || host=localhost
  printf '\033]7;file://%s%s\007' "$host" "$encoded"
}

autoload -Uz add-zsh-hook
add-zsh-hook -d precmd __zeus_report_dir 2>/dev/null
add-zsh-hook precmd __zeus_report_dir
