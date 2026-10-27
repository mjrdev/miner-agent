#!/usr/bin/env bash
# Checks miner-job inside the agent image against a local repository, with a
# fake agent instead of the real CLI. Usage: build/agent/test.sh [image]
set -euo pipefail

image=${1:-miner-agent:default}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

# A bare repository with one commit on main, readable by the image's user.
git init --quiet --bare --initial-branch=main "$tmp/origin.git"
git clone --quiet "$tmp/origin.git" "$tmp/seed" 2>/dev/null
git -C "$tmp/seed" -c user.name=t -c user.email=t@t commit --quiet --allow-empty -m init
git -C "$tmp/seed" push --quiet origin main
chmod -R a+rwX "$tmp"

# job <name> <agent shell command>: prepares a run directory and runs the job.
job() {
  local dir="$tmp/$1"
  mkdir -p "$dir" && chmod 777 "$dir"
  echo "do it" > "$dir/prompt.md"
  jq -n --arg c "$2" '["sh","-c",$c]' > "$dir/command.json"
  echo "secret-token" > "$dir/git-token"
  docker run --rm -v "$tmp:$tmp" -v "$dir:/job" \
    -e REPO_URL="file://$tmp/origin.git" -e BASE_BRANCH=main -e BRANCH=miner/t/r -e COMMIT_MESSAGE="Do it" \
    "$image" > /dev/null 2>&1 && code=0 || code=$?
  echo "$code"
}

code=$(job changes 'cat > /dev/null; echo hello > hello.txt; env > /job/agent-env; echo "{\"type\":\"result\"}"')
[ "$code" = 0 ] || fail "changes: exit $code, want 0"
ws="$tmp/changes/workspace"
[ "$(git -C "$ws" branch --show-current)" = miner/t/r ] || fail "changes: wrong branch"
[ "$(git -C "$ws" log -1 --format=%s)" = "Do it" ] || fail "changes: commit message"
grep -q hello.txt "$tmp/changes/diff.patch" || fail "changes: diff without the file"
grep -q '"result"' "$tmp/changes/events.jsonl" || fail "changes: events not written"
[ ! -e "$tmp/changes/git-token" ] || fail "changes: token file left behind"
! grep -q secret-token "$tmp/changes/agent-env" || fail "changes: token visible to the agent"
! git -C "$ws" remote -v | grep -q secret-token || fail "changes: token in the remote"

code=$(job nochanges 'cat > /dev/null')
[ "$code" = 11 ] || fail "no changes: exit $code, want 11"

code=$(job agentfails 'exit 7')
[ "$code" = 12 ] || fail "agent fails: exit $code, want 12"
[ "$(cat "$tmp/agentfails/agent_exit")" = 7 ] || fail "agent fails: agent_exit"

mkdir -p "$tmp/noclone" && chmod 777 "$tmp/noclone" && echo x > "$tmp/noclone/prompt.md" && echo '[]' > "$tmp/noclone/command.json"
docker run --rm -v "$tmp/noclone:/job" -e REPO_URL=file:///missing.git -e BASE_BRANCH=main -e BRANCH=b -e COMMIT_MESSAGE=m \
  "$image" > /dev/null 2>&1 && code=0 || code=$?
[ "$code" = 10 ] || fail "clone fails: exit $code, want 10"

echo "PASS: miner-job"
