function __zeus_encode_path
    printf '%s' "$argv[1]" | od -An -v -tx1 | awk '{ for (i = 1; i <= NF; i++) { b = toupper($i); if (b == "2F") printf "/"; else printf "%%%s", b } }'
end

function __zeus_report_dir --on-event fish_prompt
    set -l path "$PWD"
    string match -qr '^/' -- $path; or return
    set -l encoded (__zeus_encode_path "$path")
    set -l host "$HOSTNAME"
    if test -z "$host"
        set host (hostname 2>/dev/null)
    end
    if test -z "$host"
        set host localhost
    end
    printf '\e]7;file://%s%s\a' "$host" "$encoded"
end
