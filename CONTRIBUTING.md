# Contributing to MuninnDB

Thanks for your interest in contributing. This document describes our branching model and how to get changes merged.

## Branch model (Git Flow)

We use a **develop → main** flow:

| Branch    | Purpose |
|----------|---------|
| `main`   | Production-ready code. Only updated when we release. |
| `develop`| Integration branch. All feature/fix work is merged here first. |
| `feature/*`, `fix/*`, `bug/*` | Short-lived branches for your work. Merge into `develop` via PR. |

**Flow:**

1. **Start from `develop`:**  
   `git checkout develop && git pull origin develop`  
   Create your branch: `feature/my-thing`, `fix/issue-123`, or `bug/crash-on-x`.

2. **Work and push:**  
   Commit, push your branch, open a **pull request into `develop`**.

3. **CI must pass:**  
   The PR will run our CI (build, tests, Windows smoke, Python SDK). Fix any failures.

4. **Merge into `develop`:**  
   Once the PR is approved (if required) and CI is green, merge. Do **not** push directly to `develop` or `main`.

5. **Releases:**  
   When we’re ready to release, we merge `develop` into `main` (via PR) and push a version tag (e.g. `v0.2.4`). That triggers builds, PyPI publish, PHP Packagist sync, and GitHub Release.

## Summary

- **feature/fix/bug** → open PR → **develop**
- **develop** → open PR → **main** (when releasing)
- **Tag on main** → release artifacts and package publishes

## Legal

By contributing, you agree that your contributions will be licensed under the [Apache 2.0 license](LICENSE) and that you have read our [Contributor License Agreement](CLA.md).
