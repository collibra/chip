# Coding instructions
- Follow the stepdown rule when determinging the order of functions or other code constructs: we want the code to read like a top-down narrative. Every function to be followed by those at the next level of abstraction so that we can read the program, descending level of abstraction at a time as we read down the list of functions. 
- When running a command remember the shell is initialized to the workspace path by default so no need to cd into it.


# PR creation instructions
- When generating commit messages, always start the PR message with the Jira ticket reference (e.g. DEV-146292) as part of the branch name. Keep the commit message short and professional one-liner.
- Use the github CLI to create the PR. The details of the `gh pr create` command are available below.
- To find out what are the changes and generate the PR details, you must compare the current branch with the main branch.
- Any local changes that are not committed to the branch should be ignored.
- Keep the description simple and professional. Do not brag using adjectives like "enhanced" or "comprehensive" or similar.
- You must follow in the following template for the details of the PR. When using the template, you must use github compatible markdown format.

```
### Description of your changes

<!--
Please include a short summary of the changes and any relevant
motivation and context. List any dependencies that are required
for this change (i.e. Depends on merge of PR in another repo).
-->

---

### JIRA reference

<!-- DEV-123 etc. -->

---

### Impact Analysis

<!--
Please include any information about analysis that has been
performed with respect to whether or not this change may have
a potentially large or small impact on this or other codebases.
-->

---

#### Checklist

- [x] I have performed a self-review of my code
- [x] My code follows the contribution guidelines of this project
- [x] My changes generate no new warnings
```
```
gh pr create --help
Create a pull request on GitHub.

Upon success, the URL of the created pull request will be printed.

When the current branch isn't fully pushed to a git remote, a prompt will ask where
to push the branch and offer an option to fork the base repository. Use `--head` to
explicitly skip any forking or pushing behavior.

`--head` supports `<user>:<branch>` syntax to select a head repo owned by `<user>`.
Using an organization as the `<user>` is currently not supported.
For more information, see <https://github.com/cli/cli/issues/10093>

A prompt will also ask for the title and the body of the pull request. Use `--title` and
`--body` to skip this, or use `--fill` to autofill these values from git commits.
It's important to notice that if the `--title` and/or `--body` are also provided
alongside `--fill`, the values specified by `--title` and/or `--body` will
take precedence and overwrite any autofilled content.

The base branch for the created PR can be specified using the `--base` flag. If not provided,
the value of `gh-merge-base` git branch config will be used. If not configured, the repository's
default branch will be used. Run `git config branch.{current}.gh-merge-base {base}` to configure
the current branch to use the specified merge base.

Link an issue to the pull request by referencing the issue in the body of the pull
request. If the body text mentions `Fixes #123` or `Closes #123`, the referenced issue
will automatically get closed when the pull request gets merged.

By default, users with write access to the base repository can push new commits to the
head branch of the pull request. Disable this with `--no-maintainer-edit`.

Adding a pull request to projects requires authorization with the `project` scope.
To authorize, run `gh auth refresh -s project`.


USAGE
  gh pr create [flags]

ALIASES
  gh pr new

FLAGS
  -a, --assignee login       Assign people by their login. Use "@me" to self-assign.
  -B, --base branch          The branch into which you want your code merged
  -b, --body string          Body for the pull request
  -F, --body-file file       Read body text from file (use "-" to read from standard input)
  -d, --draft                Mark pull request as a draft
      --dry-run              Print details instead of creating the PR. May still push git changes.
  -e, --editor               Skip prompts and open the text editor to write the title and body in. The first line is the title and the remaining text is the body.
  -f, --fill                 Use commit info for title and body
      --fill-first           Use first commit info for title and body
      --fill-verbose         Use commits msg+body for description
  -H, --head branch          The branch that contains commits for your pull request (default [current branch])
  -l, --label name           Add labels by name
  -m, --milestone name       Add the pull request to a milestone by name
      --no-maintainer-edit   Disable maintainer's ability to modify pull request
  -p, --project title        Add the pull request to projects by title
      --recover string       Recover input from a failed run of create
  -r, --reviewer handle      Request reviews from people or teams by their handle
  -T, --template file        Template file to use as starting body text
  -t, --title string         Title for the pull request
  -w, --web                  Open the web browser to create a pull request

INHERITED FLAGS
      --help                     Show help for command
  -R, --repo [HOST/]OWNER/REPO   Select another repository using the [HOST/]OWNER/REPO format

EXAMPLES
  $ gh pr create --title "The bug is fixed" --body "Everything works again"
  $ gh pr create --reviewer monalisa,hubot  --reviewer myorg/team-name
  $ gh pr create --project "Roadmap"
  $ gh pr create --base develop --head monalisa:feature
  $ gh pr create --template "pull_request_template.md"

LEARN MORE
  Use `gh <command> <subcommand> --help` for more information about a command.
  Read the manual at https://cli.github.com/manual
  Learn about exit codes using `gh help exit-codes`
  Learn about accessibility experiences using `gh help accessibility`
```