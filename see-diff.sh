#!/bin/bash

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd)"

TMPL=~/Projects/section.io/delivery.kubernetes/shared-ingress/images/nginx-ingress-controller/nginx.tmpl

tag-list() {
    git --no-pager tag -l | \
        grep -E '^(nginx|controller)' | \
        grep -vE 'beta|alpha|nginx-0.9.0' | \
        awk -F- '{print $2 " " $1 "-" $2}' | \
        sed -E 's/^v//' | \
        sed -E 's/^0\.//' | \
        sort
}

tags() {
    tag-list | tail -n +19
}

show-git() {
    rm -rf "$DIR/by-tag" "$DIR/ordered-tags"
    mkdir -p "$DIR/by-tag"

    # skipping older nginx version 0.10.1 to 0.23.0 and will start with 0.24.0
    tags > ordered-tags

    while read -r i t; do
        git show ${t}:rootfs/etc/nginx/template/nginx.tmpl > by-tag/${i}-${t}-nginx.tmpl
    done < ordered-tags
}

create-diffs() {
    local dir="$DIR/by-tag"
    mkdir -p "$dir"
    (
        cd "$dir"
        ls -1 *.tmpl | \
            while read -r n ; do
                diff -a -y --width 200 "${n}" ${TMPL} | colordiff | expand > "diff-${n}.diff"
            done
    )
}

show-files() {
    show-git
    create-diffs
    less $DIR/by-tag/diff-*
}

"$@"
