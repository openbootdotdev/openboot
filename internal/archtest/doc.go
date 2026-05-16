// Package archtest holds architecture fitness functions for the openboot
// codebase. Each *_test.go file enforces one project-level invariant
// documented in CLAUDE.md (e.g. "exec.Command must go through internal/system",
// "raw http.NewRequest must go through internal/httputil").
//
// Rules are seeded with a baseline of current violations under baseline/. A
// rule fails only on NEW violations — existing call sites stay green. This
// implements the "quality left" principle from Martin Fowler's "Harness
// Engineering for Coding Agents": cheap computational sensors that block
// drift without forcing a refactor.
//
// To regenerate baselines after an intentional change:
//
//	ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/...
//
// See internal/archtest/README.md for how to add a new rule.
package archtest
