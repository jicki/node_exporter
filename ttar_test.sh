#!/usr/bin/env bash
# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

test_root=$(mktemp -d "${TMPDIR:-/tmp}/ttar-test.XXXXXX")
trap 'rm -rf -- "$test_root"' EXIT
script_dir=$(cd "$(dirname "$0")" && pwd)
ttar_script="$script_dir/ttar"
python_command=$(command -v python3)
bash_command=$(command -v bash)

# PATH 只包含测试指定的命令，避免宿主机的 python 别名影响分支覆盖。
function prepare_case {
    case_dir="$test_root/$1"
    mkdir -p "$case_dir/bin" "$case_dir/input/data/dir name" "$case_dir/output" || return
    local utility
    for utility in awk bash basename cat chmod ln mkdir readlink rm stat tail touch wc; do
        ln -s "$(command -v "$utility")" "$case_dir/bin/$utility" || return
    done
    cat > "$case_dir/bin/sed" <<'SED' || return
#!/bin/sh
printf 'NUL conversion unavailable\n'
SED
    chmod +x "$case_dir/bin/sed" || return

    printf '%s\n' 'EOF NULLBYTE \EOF \NULLBYTE' > "$case_dir/input/data/payload" || return
    printf '\000\000nul\nUTF-8: \344\270\255\346\226\207\n\377\r\n' >> "$case_dir/input/data/payload" || return
    printf 'last line' > "$case_dir/input/data/no-newline" || return
    printf 'carriage\r' > "$case_dir/input/data/trailing-cr" || return
    : > "$case_dir/input/data/empty" || return
    printf 'nested\n' > "$case_dir/input/data/dir name/nested" || return
    chmod 640 "$case_dir/input/data/payload" || return
    ln -s payload "$case_dir/input/data/link"
}

function add_python {
    cat > "$case_dir/bin/$1" <<'PYTHON' || return
#!/bin/sh
printf '%s\n' "${0##*/}" >> "$TTAR_TEST_LOG" || exit
exec "$TTAR_TEST_PYTHON" "$@"
PYTHON
    chmod +x "$case_dir/bin/$1"
}

function run_ttar {
    PATH="$case_dir/bin" TTAR_TEST_PYTHON="$python_command" \
        TTAR_TEST_LOG="$case_dir/python.log" "$bash_command" "$ttar_script" "$@"
}

function file_mode {
    stat -c '%a' "$1" 2>/dev/null || stat -f '%OLp' "$1"
}

function round_trip {
    local entry
    run_ttar -C "$case_dir/input" -c -f "$case_dir/archive.ttar" data || return
    run_ttar -C "$case_dir/output" -x -f "$case_dir/archive.ttar" || return
    for entry in payload no-newline trailing-cr empty 'dir name/nested'; do
        cmp "$case_dir/input/data/$entry" "$case_dir/output/data/$entry" || return
    done
    [[ -L "$case_dir/output/data/link" ]] || return
    [[ "$(readlink "$case_dir/output/data/link")" == payload ]] || return
    [[ "$(file_mode "$case_dir/output/data/payload")" == 640 ]] || return
}

function test_python_selection {
    local configuration="$1"
    local expected="$2"
    prepare_case "$configuration" || return
    case "$configuration" in
        python3-only) add_python python3 || return ;;
        python-only) add_python python || return ;;
        both)
            add_python python3 || return
            add_python python || return
            ;;
    esac
    round_trip || return
    [[ -s "$case_dir/python.log" ]] || return
    local interpreter
    while IFS= read -r interpreter; do
        if [[ "$interpreter" != "$expected" ]]; then
            echo "Unexpected interpreter selection: $interpreter"
            return 1
        fi
    done < "$case_dir/python.log"
}

function test_no_python {
    prepare_case no-python || return
    local status=0
    run_ttar -C "$case_dir/input" -c -f "$case_dir/archive.ttar" data \
        > "$case_dir/error.log" 2>&1 || status=$?
    [[ "$status" -eq 2 ]] || return
    [[ ! -e "$case_dir/archive.ttar" ]] || return
    grep -q 'ERROR.*Python.*not found' "$case_dir/error.log"
}

function test_sed_without_python {
    prepare_case sed-without-python || return
    # 这里只模拟外部 sed 的能力探测；列表操作不调用内容转换滤镜。
    cat > "$case_dir/bin/sed" <<'SED' || return
#!/bin/sh
printf '\000\n'
SED
    cat > "$case_dir/list.ttar" <<'ARCHIVE' || return
Directory: data
Path: data/file
Lines: 1
content
Mode: 644
ARCHIVE
    run_ttar -t -f "$case_dir/list.ttar" > "$case_dir/list.txt" || return
    printf 'data/\ndata/file\n' > "$case_dir/expected.txt" || return
    cmp "$case_dir/expected.txt" "$case_dir/list.txt"
}

failures=0
for configuration in python3-only python-only both; do
    expected=python3
    if [[ "$configuration" == python-only ]]; then
        expected=python
    fi
    if test_python_selection "$configuration" "$expected"; then
        echo "PASS: $configuration"
    else
        echo "FAIL: $configuration"
        failures=$((failures + 1))
    fi
done
for test_case in test_no_python test_sed_without_python; do
    if "$test_case"; then
        echo "PASS: $test_case"
    else
        echo "FAIL: $test_case"
        failures=$((failures + 1))
    fi
done
if [[ "$failures" -ne 0 ]]; then
    echo "$failures test cases failed"
    exit 1
fi
