# Files and live reload

## The tree

The sidebar shows the directory served as a file tree, with a file finder (press `t`) and a "Markdown only" filter. A directory's page lists its files and shows its README, as GitHub does.

Every file on disk is shown, including what git ignores; those are marked `local`. Directories git ignores, such as a `.venv`, show in the tree without their files until you open them, and Hide ignored directories in the settings keeps them out of sight. Names given to `--skip` are left out of the tree altogether, and gh-mini does not read or watch them.

## Live reload

The page you are reading reloads when its file changes, and the tree when files are added or removed. `--no-reload` turns this off.

On macOS the whole directory is watched through FSEvents. Elsewhere, directories git ignores are watched only while you look at a page in them.

A new symlink that points to another directory shows in the tree only when its directory changes otherwise too.
