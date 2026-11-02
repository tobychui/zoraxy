#!/bin/bash
# Run every Go test in this module and print a pass / fail / skip report to STDOUT.
#
# Usage:
#   ./run_tests.sh                     # test ./...
#   ./run_tests.sh -short -timeout 5m  # extra arguments are passed to "go test"
#   PKGS=./mod/auth/... ./run_tests.sh # limit the packages under test
#   VERBOSE=1 ./run_tests.sh           # also list every passed test
#
# Exit code is 0 when everything passed, 1 otherwise.

cd "$(dirname "$0")" || exit 1

PKGS="${PKGS:-./...}"
VERBOSE="${VERBOSE:-0}"

if ! command -v go >/dev/null 2>&1; then
    echo "go is not installed or not in PATH" >&2
    exit 1
fi

if [ -t 1 ]; then
    C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BOLD=$'\033[1m'; C_RESET=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BOLD=""; C_RESET=""
fi

RESULT_FILE="$(mktemp)"
trap 'rm -f "$RESULT_FILE"' EXIT

echo "Running go test -json -count=1 $* $PKGS ..."
START_TIME=$(date +%s)
# shellcheck disable=SC2086
go test -json -count=1 "$@" $PKGS >"$RESULT_FILE" 2>&1
GO_EXIT=$?
END_TIME=$(date +%s)

awk \
    -v RED="$C_RED" -v GREEN="$C_GREEN" -v YELLOW="$C_YELLOW" -v BOLD="$C_BOLD" -v RESET="$C_RESET" \
    -v VERBOSE="$VERBOSE" -v WALL="$((END_TIME - START_TIME))" -v GO_EXIT="$GO_EXIT" '
function jstr(line, key,    re, s) {
    re = "\"" key "\":\"([^\"\\\\]|\\\\.)*\""
    if (!match(line, re)) return ""
    s = substr(line, RSTART + length(key) + 4, RLENGTH - length(key) - 5)
    return unesc(s)
}
function jnum(line, key,    re) {
    re = "\"" key "\":[0-9.eE+-]+"
    if (!match(line, re)) return ""
    return substr(line, RSTART + length(key) + 3, RLENGTH - length(key) - 3)
}
function unesc(s) {
    gsub(/\\\\/, "\001", s)
    gsub(/\\n/, "\n", s)
    gsub(/\\t/, "\t", s)
    gsub(/\\r/, "", s)
    gsub(/\\"/, "\"", s)
    gsub(/\\u003c/, "<", s)
    gsub(/\\u003e/, ">", s)
    gsub(/\\u0026/, "\\&", s)
    gsub(/\\u001b\[[0-9;]*m/, "", s)
    gsub(/\001/, "\\", s)
    return s
}
function dur(t) {
    return (t == "") ? "did not finish" : sprintf("%.2fs", t)
}
function indent(s, pad,    out, n, i, parts) {
    sub(/\n+$/, "", s)
    n = split(s, parts, "\n")
    out = ""
    for (i = 1; i <= n; i++) out = out pad parts[i] "\n"
    return out
}

!/^\{/ {
    if ($0 != "") raw = raw $0 "\n"
    next
}

{
    action = jstr($0, "Action")
    pkg = jstr($0, "Package")
    test = jstr($0, "Test")

    if (action == "build-output") {
        buildout = buildout jstr($0, "Output")
        next
    }
    if (action == "build-fail") next
    if (pkg == "") next

    if (!(pkg in pkgseen)) { pkgseen[pkg] = 1; pkgorder[++npkg] = pkg }

    if (test == "") {
        if (action == "output") pkgout[pkg] = pkgout[pkg] jstr($0, "Output")
        else if (action == "pass" || action == "fail" || action == "skip") {
            pkgres[pkg] = action
            pkgtime[pkg] = jnum($0, "Elapsed")
        }
        next
    }

    key = pkg SUBSEP test
    if (!(key in testseen)) { testseen[key] = 1; testorder[++ntest] = key }

    if (action == "output") testout[key] = testout[key] jstr($0, "Output")
    else if (action == "pass" || action == "fail" || action == "skip") {
        testres[key] = action
        testtime[key] = jnum($0, "Elapsed")
    }
}

END {
    npass = nfail = nskip = nrun = 0
    for (i = 1; i <= ntest; i++) {
        key = testorder[i]
        r = testres[key]
        if (r == "pass") npass++
        else if (r == "fail") nfail++
        else if (r == "skip") nskip++
        else { nrun++; testres[key] = "fail"; nfail++ }
        split(key, kp, SUBSEP)
        if (testres[key] == "fail") pkgfailed[kp[1]]++
    }

    ppass = pfail = pnotest = 0
    print ""
    print BOLD "==================== PACKAGES ====================" RESET
    for (i = 1; i <= npkg; i++) {
        p = pkgorder[i]
        r = pkgres[p]
        if (r == "") r = "fail"
        t = " (" dur(pkgtime[p]) ")"
        if (r == "skip" || pkgout[p] ~ /\[no test files\]/) {
            pnotest++
            if (VERBOSE == "1") printf "  %s%-6s%s %s [no test files]\n", YELLOW, "NONE", RESET, p
        } else if (r == "pass") {
            ppass++
            printf "  %s%-6s%s %s%s\n", GREEN, "PASS", RESET, p, t
        } else {
            pfail++
            printf "  %s%-6s%s %s%s\n", RED, "FAIL", RESET, p, t
        }
    }
    if (VERBOSE != "1" && pnotest > 0)
        printf "  (%d package(s) without test files hidden, use VERBOSE=1 to list them)\n", pnotest

    if (VERBOSE == "1") {
        print ""
        print BOLD "==================== PASSED TESTS ====================" RESET
        for (i = 1; i <= ntest; i++) {
            key = testorder[i]
            if (testres[key] != "pass") continue
            split(key, kp, SUBSEP)
            printf "  %sPASS%s %s  %s (%s)\n", GREEN, RESET, kp[1], kp[2], dur(testtime[key])
        }
    }

    if (nskip > 0) {
        print ""
        print BOLD "==================== SKIPPED TESTS ====================" RESET
        for (i = 1; i <= ntest; i++) {
            key = testorder[i]
            if (testres[key] != "skip") continue
            split(key, kp, SUBSEP)
            reason = testout[key]
            gsub(/=== (RUN|PAUSE|CONT)[^\n]*\n/, "", reason)
            gsub(/--- SKIP[^\n]*\n/, "", reason)
            gsub(/^[ \t\n]+|[ \t\n]+$/, "", reason)
            gsub(/\n[ \t]*/, " | ", reason)
            printf "  %sSKIP%s %s  %s%s\n", YELLOW, RESET, kp[1], kp[2], (reason != "" ? "  -> " reason : "")
        }
    }

    if (nfail > 0 || pfail > 0) {
        print ""
        print BOLD "==================== FAILURES ====================" RESET
        for (i = 1; i <= ntest; i++) {
            key = testorder[i]
            if (testres[key] != "fail") continue
            split(key, kp, SUBSEP)
            printf "\n%sFAIL%s %s  %s (%s)\n", RED, RESET, kp[1], kp[2], dur(testtime[key])
            out = testout[key]
            gsub(/=== (RUN|PAUSE|CONT)[^\n]*\n/, "", out)
            if (out != "") printf "%s", indent(out, "    | ")
        }
        for (i = 1; i <= npkg; i++) {
            p = pkgorder[i]
            if (pkgres[p] == "pass" || pkgres[p] == "skip" || pkgfailed[p] > 0) continue
            printf "\n%sFAIL%s %s  (package failed outside of a test: build error, panic or TestMain)\n", RED, RESET, p
            if (pkgout[p] != "") printf "%s", indent(pkgout[p], "    | ")
        }
        if (buildout != "") {
            print ""
            print BOLD "Build output:" RESET
            printf "%s", indent(buildout, "    | ")
        }
    }

    if (raw != "") {
        print ""
        print BOLD "Non-JSON output from go test:" RESET
        printf "%s", indent(raw, "    | ")
    }

    print ""
    print BOLD "==================== SUMMARY ====================" RESET
    printf "  Packages : %s%d passed%s, %s%d failed%s, %d without tests\n", GREEN, ppass, RESET, (pfail ? RED : ""), pfail, RESET, pnotest
    printf "  Tests    : %s%d passed%s, %s%d failed%s, %s%d skipped%s (%d total, subtests included)\n", \
        GREEN, npass, RESET, (nfail ? RED : ""), nfail, RESET, (nskip ? YELLOW : ""), nskip, RESET, ntest
    if (nrun > 0) printf "  %s%d test(s) never finished (timeout or crash), counted as failed%s\n", RED, nrun, RESET
    printf "  Duration : %ds\n", WALL

    if (nfail > 0 || pfail > 0 || GO_EXIT != 0) {
        print "  Result   : " RED BOLD "FAILED" RESET
        exit 1
    }
    print "  Result   : " GREEN BOLD "PASSED" RESET
    exit 0
}
' "$RESULT_FILE"
