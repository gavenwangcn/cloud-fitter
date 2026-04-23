#!/usr/bin/env bash
set -euo pipefail
rm -rf gen/*
buf generate
