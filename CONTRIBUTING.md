# Contributing to CHIP

First off, thank you for considering contributing! We're excited you're interested in helping out. This project, like all open-source projects, is built and maintained by a community of developers just like you.

We welcome all kinds of contributions:
* Reporting a bug
* Submitting a fix
* Proposing new features
* Improving documentation

## 💬 How to Get Help or Ask Questions

If you have a question or aren't sure how to contribute, the best way to get in touch is to [**open an issue**](https://github.com/collibra/chip/issues/new/choose) on our GitHub repository.

## 🤝 How to Contribute

We follow a standard branch-based workflow.

1.  **Clone** the repository to your local machine:
    ```bash
    git clone https://github.com/collibra/chip.git
    cd chip
    ```
2.  **Create a new branch** for your changes. Please use a descriptive name:
    ```bash
    # For a new feature
    git checkout -b feat/my-new-feature
    # For a bug fix
    git checkout -b fix/issue-123
    ```
3.  **Make your changes** to the code.
4.  **Run tests and linters** to ensure your code is ready.
    ```bash
    # Run tests (with the race detector!)
    go test -race ./...

    # Run the linter (we use golangci-lint)
    golangci-lint run
    ```
5.  **Commit your changes** using our commit message convention (see below).
6.  **Push** your branch to the repository:
    ```bash
    git push origin feat/my-new-feature
    ```
7.  **Open a Pull Request (PR)** from your branch to our `main` branch.
8.  A maintainer will review your PR, and we'll work with you to get it merged.

---

## 📜 Our Commit Message Convention

This is one of the most important parts of contributing. We use **Conventional Commits** to keep our commit history clean, readable, and to automate our release process and CHANGELOG generation.

Your commit messages **must** follow this specification.

### Format

Each commit message consists of a **header**, a **body**, and a **footer**.

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### 1. Header

The header is the only mandatory part.

* **`<type>`:** This describes the *kind* of change you're making.
    * **`feat`:** A new feature (this will trigger a `MINOR` version bump).
    * **`fix`:** A bug fix (this will trigger a `PATCH` version bump).
    * **`docs`:** Changes to documentation only.
    * **`style`:** Code style changes (formatting, etc.) that don't affect logic.
    * **`refactor`:** A code change that neither fixes a bug nor adds a feature.
    * **`test`:** Adding or correcting tests.
    * **`chore`:** Changes to the build process, CI, or other tooling.
    * **`ci`:** Changes to our CI configuration files and scripts.

* **`[optional scope]`:** A word in parentheses to specify the part of the codebase you're changing (e.g., `(api)`, `(parser)`, `(cmd)`).

* **`<description>`:** A short, clear description of the change (under 72 characters) in the imperative, present tense.
    * **Good:** `feat(api): add user login endpoint`
    * **Bad:** `feat(api): added the user login endpoint`

### 2. Body (Optional)

The body is for *why* you made the change, not just *what* you changed. Explain the previous behavior, the new behavior, and your reasoning.

### 3. Footer (Optional) & Breaking Changes

The footer is used to reference issues (e.g., `Fixes #123`) or, most importantly, to signal a **Breaking Change**.

> **🚨 How to Signal a Breaking Change**
>
> A breaking change **must** be signaled to trigger a `MAJOR` version bump.
>
> 1.  Add an `!` after the type: `feat!(api): ...`
> 2.  **AND/OR** add a `BREAKING CHANGE:` footer at the very bottom of the commit.
>
> ```
> feat!(config): remove support for TOML config files
>
> BREAKING CHANGE: Support for .toml configuration files has been
> removed. All users must migrate to .yaml or .json files.
> ```

### Examples

* **A simple fix:**
    ```
    fix: correct nil pointer dereference on user logout
    ```
* **A new feature with a scope:**
    ```
    feat(api): add rate limiting to all /v1 endpoints
    ```
* **A refactor with a body:**
    ```
    refactor(database): switch from pq to pgx driver
    
    The pq driver is no longer actively maintained. pgx offers
    better performance and native context support. This commit
    migrates all database calls.
    ```
* **A documentation change:**
    ```
    docs(readme): update installation instructions
    ```
* **A Breaking Change:**
    ```
    fix!(auth): enforce stricter password complexity
    
    BREAKING CHANGE: Passwords now require a minimum of 12
    characters, one uppercase, one number, and one special character.
    ```
