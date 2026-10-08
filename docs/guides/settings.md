# Settings

The gear button opens a menu with the keyboard shortcuts (or press `?`), an About dialog that tells the version and the directory served, and the settings (or press `,`).

## The settings

| Setting | Default | |
|---|---|---|
| Theme | `github`, or `--theme` | The built-in `github`, `nord` and `tokyo-night`, or one you write (see [Themes](themes.md)) |
| Mode | Auto | Light, dark, or Auto to follow the system |
| Full width | off | Let the page take the width of the window rather than a column |
| Wrap code | off | Wrap long lines of a source file rather than scroll them |
| HTML preview | off | Open HTML files rendered, with their scripts, rather than as code (see [HTML previews](html-previews.md)) |
| Language switch | on | Switch between a Markdown file and its translations (see [Translations](translations.md)) |
| Hide ignored directories | off | Leave directories git ignores out of the tree, the listings and the file finder |
| Changes | on | Show what changed since the last commit (see [Changes](changes.md)); the settings below do nothing while it is off |
| Marks in the tree | Letter and color | How the tree tells a changed file: a letter and its name in color, the color alone, or nothing |
| Untracked files | on | Count files git does not track yet as changed |
| Changed words | on | Mark the words that differ in a line replaced by another |
| Ignore whitespace | off | Leave out changes of whitespace alone from diffs, as `git diff -w` does |
| Open a changed file at | The file | What the page of a changed file shows first: the file, or its diff |
| Avatars | on | Show the author of a file's last commit with their picture on GitHub, which asks GitHub for it by their email |

The settings are kept per browser, and hold for every gh-mini on `localhost`.

## Settings and flags

Flags set up what a gh-mini serves and what it can do. The settings are how you like to see it, within that.

A flag that gives a setting's default, as `--theme` does, gives way to what you pick in the settings. A setting for something the command line turned off, such as HTML preview when no port could be had for it, is shown off with why.

gh-mini answers only when it is opened at `localhost`, at an IP address, at the name given to `--host`, or, with `--host 0.0.0.0`, at the machine's name. A page on another site that makes its own name lead to your machine, by DNS rebinding, so gets nothing.
