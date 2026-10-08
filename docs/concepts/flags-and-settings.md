# Flags and settings

gh-mini takes options in two places: flags on the command line, and settings in the page's settings dialog. They answer different questions, and every option is one or the other, never both.

- A flag is for the gh-mini: what it serves and where, what it can do, and facts of the directory served. It sets up the world a page is shown in, and the page cannot change it.
- A setting is for the viewer: how they like to see what is served, within that world.

## Where an option goes

Ask whom the option is for.

- Does it change what gh-mini reads, watches, listens on or can do? It is a flag. `--port`, `--host`, `--preview-port`, `--no-reload`, `--no-changes`, `--skip` and `--theme-dir` are.
- Does it follow from how the directory served is laid out, so that another directory may want another value? It is a flag. `--translations` is: a repository names its translations its own way, whoever reads it.
- Is it how one person likes to read, whatever the directory? It is a setting. Theme, Mode, Full width, Wrap code, HTML preview, Language switch, Hide ignored directories, Avatars and the settings of Changes are.

The settings live in one cookie that every gh-mini on localhost shares, whatever its port (see [settings.go](../../internal/server/settings.go)). A pick made while reading one directory holds for all the others. That is right for a viewer's taste and wrong for a fact of one directory, which is why translations are named by a flag and not picked in the dialog.

## How they meet

A flag may give a setting's default, as `--theme` does for Theme. It is only the value a viewer sees before picking one; what the viewer picks wins, on every gh-mini. A flag that does this says so in its usage, such as "until one is picked".

A flag may turn off what a setting is for. HTML preview does nothing when gh-mini has no port for previews, Language switch does nothing with `--translations off`, and Changes does nothing with `--no-changes`. Such a setting stays in the dialog, so that a viewer who knows it from another gh-mini finds it, but it is shown off and disabled, with why. Give it an `Unavailable` in settingDefs that returns the reason.

The dialog never offers a choice the world does not have, and a flag never overrides a pick the viewer made.

## Alike, on either side

`--skip` and Hide ignored directories both keep things out of the tree, but on different sides. `--skip` leaves names out of what gh-mini reads and watches, so that they are not there at all. Hide ignored directories keeps directories git ignores out of sight for one viewer, while gh-mini still reads and watches them.

## Settings under another

A setting may tell more of a toggle above it, as Ignore whitespace does of Changes: it does nothing while that is off, and the dialog shows it so. Give it a `Parent`, the key of that toggle, which comes before it in settingDefs. Keep such settings to those that mean nothing without the one above; Wrap code wraps diffs too, but it is of files, and stays with them.

## Adding one

- A flag: parseArgs in [main.go](../../main.go), passed to the server in `server.Options`, and a row in the README's Flags table.
- A setting: an entry in settingDefs in [settings.go](../../internal/server/settings.go); the comment at the top of that file tells how its value reaches the page. Add a row to the table in [Settings](../guides/settings.md) too.
