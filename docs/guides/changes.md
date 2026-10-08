# Changes

When the directory served is in a git repository, gh-mini shows what changed since the last commit, as `git status` and `git diff` tell it: files edited, staged, added, deleted and renamed, and files git does not track yet.

On a branch other than the base, origin's default branch, it shows what changed since the branch left the base instead, as a pull request does: the commits since and what is not committed yet together (see [Since the base](#since-the-base)).

## Where they show

The top bar counts the files changed and the lines added and removed, next to the branch, and leads to the Changes page below. It is gone while nothing is changed.

In the tree, a letter after a file's name tells how it changed, and the name takes its color:

| Letter | |
|---|---|
| `M` | modified |
| `A` | added |
| `D` | deleted |
| `R` | renamed |
| `U` | untracked: git does not track it yet |
| `C` | in a merge conflict |

A directory with changes under it has a dot, so that a folded one tells where to look. Changed files, in the menu beside the finder, leaves the tree with only the changed files, and opens the directories that hold them.

A directory's page tells how many lines each file changed, and for a directory, those of the files under it. A deleted file stays listed, struck through, until the deletion is committed.

## A file's page

A changed file's page says how it changed, and whether that is staged, above the file, with a link to it on the Changes page. A Diff view joins Preview and Code, with the lines it changed on its tab; a source file, which has no views otherwise, gets Code and Diff. The diff is that of everything changed since the last commit, staged or not.

A file deleted still has a page, of its deletion, until the deletion is committed: follow it from the tree or a directory's page.

## The Changes page

The count in the top bar leads to the Changes page, `/_mini/changes`, which shows the diff of every file changed, as a pull request on GitHub does: highlighted, with the lines' numbers before and after. Where a line was replaced by another much like it, the words that differ are marked. A file's header folds it, tells how many lines it changed, and leads to its page.

All shows everything changed since the last commit. Staged and Unstaged show the two parts of it, what `git diff --cached` and `git diff` show, and Untracked the files git does not track. A file changed in both is tagged staged and unstaged.

A binary file is listed without its diff. A diff longer than 1500 lines, or past 20000 lines on the page, is folded, and shown on a page of its own when opened. The page follows the files and git as the tree does.

On a branch other than the base, Since main and Uncommitted pick between what changed since the branch left the base and what changed since the last commit. Since main lists the commits since, those no remote branch has marked not pushed, and tags each file committed, uncommitted or both. Uncommitted is the page as on the base, with its own tabs.

Untracked is not ignored: a file git ignores is marked `local` (see [Files and live reload](files.md)) and is never counted as changed.

## Since the base

The base is the branch origin's HEAD names, the default branch the repository was cloned with, or else origin's `main` or `master`. It is origin's, `origin/main`, and not the local `main`, which a worktree seldom updates.

On a branch other than the base, the top bar, the tree, directories' pages and files' pages show what changed since the branch left the base: the working tree against the commit where it left it, as `git diff $(git merge-base origin/main HEAD)` tells it, with untracked files. A file committed and then changed back is not listed. A changed file's page tells whether it was committed on the branch, and its diff is that since the base. The commit where the branch left the base is found again as it moves, as when the base is merged into the branch.

The branch in the top bar tells how many commits it is ahead of the base and behind it, the second in another color, as a sign that the branch may want a rebase. The tab's title ends with the branch, so that the tabs of a repository's worktrees tell apart.

The branch opens a panel, as `b` does, that tells where the branch left the base and whether its commits are pushed. gh-mini does not fetch: how far behind the base the branch is, is as of the last fetch. A commit, a fetch or a push shows without reloading.

On the base itself, outside a repository with an origin, and with `--no-changes`, the branch shows as before, a name alone, and the changes are those since the last commit.

## Settings

The Changes section of the settings turns all this off, and tells how it shows: the marks in the tree, whether untracked files count, the words marked in a line, whitespace left out of diffs, and whether a changed file opens at its diff (see [Settings](settings.md)). `--no-changes` keeps gh-mini from reading the changes at all, as for a repository too large for `git status` to be quick.

## What is counted

The lines are those `git diff HEAD` counts, what is staged and what is not together, and those of untracked files. A binary file is counted as changed, without lines. Of untracked files, the lines of the first 2000 are counted, and none of a file over 1 MB.

Names given to `--skip` are not looked at. A repository inside the one served, such as one cloned into it, is untracked as a whole, and its files are not listed.

## When they change

Editing a file, staging it with `git add`, committing, and switching branches all show without reloading: gh-mini follows git's index as well as the files. A directory's page reloads when what it lists changed. With `--no-reload`, the status is read again on the next page.
