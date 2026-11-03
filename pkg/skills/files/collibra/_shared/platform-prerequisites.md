# Platform command-line tools

How to check, and only if needed install, sign in to and choose the profile for a platform's command-line tool (for Databricks, the `databricks` command). Shared by the skills that need one, so they behave the same.

## Only when a step needs it, never at the start

The platform's command-line tool is needed for two things only:

- reading how the agent is deployed, during the scan, and
- deploying the agent with its rules file.

Look at the tool the first time one of these comes up, never before. A customer who only writes agent contracts, or whose agent isn't on a supported platform, is never asked about it.

## The four parts

For the platform found:

1. **Check (always, and again after every "continue").** Run the version command, and list the signed-in profiles without contacting any workspace (the table below). Read-only, and it costs nothing. This is also how you notice a tool someone installed later, for example the customer's IT team.
2. **Install (optional).** Only an offer, and only when the check found no tool. Show the vendor's official command for the customer's system first (the table below), then ask, word for word: "Shall I install `<tool>` with the command above? Reply "yes, install" to proceed. Anything else and I'll carry on without it." Install only after a clear "yes, install". Never pipe a downloaded script into a shell. If the customer says no, can't install software, or their IT team will do it, offer to write a short message for IT, and carry on without the tool.
3. **Sign in (optional).** Only when you are about to use the tool and the chosen profile isn't signed in. The customer runs the sign-in command, because it opens a browser and you never handle tokens or secrets. In Claude Code, suggest typing `! <command>` so it runs in the session.
4. **Confirm the account (optional, but always before the first use).** Show the profiles with their workspace addresses; the customer picks one, never you, even when there is only one. Then run one read-only "who am I" call with that profile, and say which account will be used. Every look-up stays read-only.

A sign-in or permission error is not retried on your own. Never print, store or write a token, password or connection string, and never read credential files.

## Without the tool, nothing is lost, only moved

| Needs the platform | With the tool, after the customer agrees | Without it |
|---|---|---|
| How the agent is deployed, during the scan | Read it, read-only | The scan describes the agent from its code only, and its summary says the deployment wasn't looked up. Collibra still gets the agent, but no AI Endpoint until a later scan with the tool |
| Deploying with the rules file | Give the exact commands with the chosen profile for the customer to run | The customer deploys in the platform's web interface or runs the commands themselves, or the steps go into the note for whoever deploys |
| Checking the agent runs the new rules | Not needed: this reads Collibra | The same |

## Python 3.12 or later

The Guardian SDK needs Python 3.12 or later in the agent's project environment. Check it, and help when it's missing,
the same way as a platform tool:

1. **Check (always):** the project environment's Python (`<project env>/bin/python --version`, or `python3 --version`
   when there's no environment yet), and whether a newer Python is already on the computer (`uv python list` when `uv`
   is there, `python3.12 --version`, `python3.13 --version`).
2. **A newer Python is already there:** offer to create the project's environment with it, as part of the step 1
   changes (one "yes, change my code" for all of them). Never change the customer's system Python.
3. **None is there (optional install):** offer the official command for the customer's system, shown first, and run it
   only after "yes, install":

   | Customer has | Command |
   |---|---|
   | `uv` (no admin rights needed) | `uv python install 3.12` |
   | macOS with Homebrew | `brew install python@3.12` |
   | Windows | `winget install Python.Python.3.12` |
   | Anything else | the installer from python.org, which the customer runs |

   If the customer can't install software, or IT will do it, write the message for IT and pause until "continue".
4. **Where the agent is deployed** must also run Python 3.12 or later. Check the platform's setting where the project
   shows it; otherwise put it in the note for whoever deploys.

## Commands by platform

| Platform | Check | Install (macOS / Windows) | Sign in | Confirm the account |
|---|---|---|---|---|
| Databricks | `databricks --version`; profiles: `databricks auth profiles` | `brew tap databricks/tap && brew install databricks` / `winget install Databricks.DatabricksCLI` | `databricks auth login --host <workspace URL> --profile <name>` | `databricks auth profiles` (the profile shows Valid), then `databricks current-user me -p <name>` |
| AWS | `aws --version` | `brew install awscli` / `winget install Amazon.AWSCLI` | `aws configure sso`, then `aws sso login --profile <name>` | `aws sts get-caller-identity --profile <name>` |
| Azure | `az --version` | `brew install azure-cli` / `winget install Microsoft.AzureCLI` | `az login` | `az account show` |
| Google Cloud | `gcloud --version` | `brew install --cask google-cloud-sdk` / the vendor's installer | `gcloud auth login` | `gcloud auth list` |
| Snowflake | `snow --version` | The vendor's Snowflake CLI installer | `snow connection add` | `snow connection test` |

On Linux, or when these commands don't fit the customer's system, point to the vendor's own installation page instead of guessing a command. Only Databricks is used for look-ups and deploy steps today; the other rows are for the check and the install offer.

**The Databricks workspace id** is the digits after `?o=` in the workspace URL. Read it from the customer's own URL or profile. Never accept the example id from any documentation.
