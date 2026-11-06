__zeus_encode_path() {
  LC_ALL=C printf '%s' "$1" |
    LC_ALL=C od -An -v -tx1 |
    LC_ALL=C awk '{ for (i = 1; i <= NF; i++) { b = toupper($i); if (b == "2F") printf "/"; else printf "%%%s", b } }'
}

__zeus_report_dir() {
  local path encoded host
  path=${PWD}
  case "$path" in /*) ;; *) return ;; esac
  encoded=$(__zeus_encode_path "$path") || return
  host=${HOSTNAME:-}
  if [ -z "$host" ]; then host=$(hostname 2>/dev/null) || host=localhost; fi
  printf '\033]7;file://%s%s\007' "$host" "$encoded"
}

__zeus_prompt_decl=$(declare -p PROMPT_COMMAND 2>/dev/null || :)
case "$__zeus_prompt_decl" in
  "declare -a "*)
    __zeus_prompt_has_hook=0
    for __zeus_prompt_entry in "${PROMPT_COMMAND[@]}"; do
      [ "$__zeus_prompt_entry" = "__zeus_report_dir" ] && __zeus_prompt_has_hook=1
    done
    [ "$__zeus_prompt_has_hook" = 1 ] || PROMPT_COMMAND+=(__zeus_report_dir)
    ;;
  *)
    case "${PROMPT_COMMAND:-}" in
      *__zeus_report_dir*) ;;
      *) PROMPT_COMMAND="${PROMPT_COMMAND:+${PROMPT_COMMAND};}__zeus_report_dir" ;;
    esac
    ;;
esac
unset __zeus_prompt_decl __zeus_prompt_has_hook __zeus_prompt_entry
