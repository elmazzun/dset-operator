#!/bin/sh

#set -x 

# Gracefully handle the TERM signal sent when deleting the DaemonSet
trap exit TERM

HOSTS="hosts-to-ping.txt"

[ -f "$HOSTS" ] || { echo "file not found, quitting"; exit 1; }

test_reachability() {
    local res=0
    while read -r HOST; do
        ping -c 1 "$HOST" &> /dev/null
        res=$?
        if [[ $res -ne 0 ]]; then
            echo "$HOST not reachable"
            return $res
        fi
        echo "$HOST reachable"
    done < "$HOSTS"
}

if test_reachability; then
    echo "PING OK"
else
    echo "PING FAILED"
    exit 1
fi
