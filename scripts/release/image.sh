#!/usr/bin/env bash
set -euo pipefail
: "${RELEASE_COMMIT:?}"
: "${GH_TOKEN:?}"
[[ "$RELEASE_COMMIT" =~ ^[a-f0-9]{40}$ ]]
image="ghcr.io/iiwish/semlia:${RELEASE_COMMIT}"
printf '%s' "$GH_TOKEN" | docker login ghcr.io --username "${GITHUB_ACTOR:?}" --password-stdin
trap 'docker logout ghcr.io >/dev/null' EXIT
docker build --file deploy/local/Dockerfile --label "org.opencontainers.image.revision=$RELEASE_COMMIT" --label org.opencontainers.image.source=https://github.com/iiwish/semlia --tag "$image" .
docker push "$image"
mkdir -p build/release
docker image inspect "$image" --format '{{json .RepoDigests}}' | node -e '
let raw=""; process.stdin.on("data", b=>raw+=b); process.stdin.on("end",()=>{
 const image=JSON.parse(raw).find(x=>/^ghcr.io\/iiwish\/semlia@sha256:[a-f0-9]{64}$/.test(x));
 if(!image) throw Error("Missing immutable image digest");
 require("node:fs").writeFileSync("build/release/image.json",JSON.stringify({image,commit:process.env.RELEASE_COMMIT,platform:"linux/amd64"}));
});'
