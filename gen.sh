#!/usr/bin/env bash
set -euo pipefail
rm -rf gen/*
# 无 buf.lock 时（如新克隆）需先解析 buf.yaml 中的远程依赖，否则 google/api/annotations.proto 等找不到
if [[ ! -f buf.lock ]]; then
	buf mod update
fi
buf generate
