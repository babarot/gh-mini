# Changes

When the directory served is in a git repository, gh-mini shows what changed since the last commit, as `git status` and `git diff` tell it: files edited, staged, added, deleted and renamed, and files git does not track yet.

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

A directory with changes under it has a dot, so that a folded one tells where to look. "Changed only" leaves the tree with only the changed files, and opens the directories that hold them.

A directory's page tells how many lines each file changed, and for a directory, those of the files under it. A deleted file stays listed, struck through, until the deletion is committed.

## A file's page

A changed file's page says how it changed, and whether that is staged, above the file, with a link to it on the Changes page. A Diff view joins Preview and Code, with the lines it changed on its tab; a source file, which has no views otherwise, gets Code and Diff. The diff is that of everything changed since the last commit, staged or not.

A file deleted still has a page, of its deletion, until the deletion is committed: follow it from the tree or a directory's page.

## The Changes page

The count in the top bar leads to the Changes page, `/_mini/changes`, which shows the diff of every file changed, as a pull request on GitHub does: highlighted, with the lines' numbers before and after. A file's header folds it, tells how many lines it changed, and leads to its page.

All shows everything changed since the last commit. Staged and Unstaged show the two parts of it, what `git diff --cached` and `git diff` show, and Untracked the files git does not track. A file changed in both is tagged staged and unstaged.

A binary file is listed without its diff. A diff longer than 1500 lines, or past 20000 lines on the page, is folded, and shown on a page of its own when opened. The page follows the files and git as the tree does.

Untracked is not ignored: a file git ignores is marked `local` (see [Files and live reload](files.md)) and is never counted as changed.

## What is counted

The lines are those `git diff HEAD` counts, what is staged and what is not together, and those of untracked files. A binary file is counted as changed, without lines. Of untracked files, the lines of the first 2000 are counted, and none of a file over 1 MB.

Names given to `--skip` are not looked at. A repository inside the one served, such as one cloned into it, is untracked as a whole, and its files are not listed.

## When they change

Editing a file, staging it with `git add`, committing, and switching branches all show without reloading: gh-mini follows git's index as well as the files. A directory's page reloads when what it lists changed. With `--no-reload`, the status is read again on the next page.
