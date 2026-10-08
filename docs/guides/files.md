# Files and live reload

## The tree

The sidebar shows the directory served as a file tree, with a file finder (press `t`) and a "Markdown only" filter. A directory's page lists its files and shows its README, as GitHub does.

The finder takes a file whose path holds the letters typed in their order, as `rdme` finds `README.md`. The arrow keys move through the tree: up and down between the rows, right to open a directory, left to fold it. Directories you open stay open from page to page, and Collapse all folds them; those opened only as the way to the page shown do not stay.

Every file on disk is shown, including what git ignores; those are marked `local`. What a repository inside the directory ignores, such as one cloned into it, counts too. Directories git ignores, such as a `.venv`, show in the tree without their files until you open them, and Hide ignored directories in the settings keeps them out of sight. Names given to `--skip` are left out of the tree altogether, and gh-mini does not read or watch them.

## Live reload

The page you are reading reloads when its file changes, and the tree when files are added or removed. `--no-reload` turns this off. A page that missed changes, as one gone back to or one open while the server restarted, catches up with them. A change to the `.gitignore` of a directory above the one served, or to `.git/info/exclude`, is followed too.

On macOS the whole directory is watched through FSEvents. Elsewhere, directories git ignores are watched only while you look at a page in them.

A new symlink that points to another directory shows in the tree only when its directory changes otherwise too.
