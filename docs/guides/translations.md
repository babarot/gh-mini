# Translations

A Markdown file and its translations get a language switch, and a directory's README is shown in the language picked last.

## Layouts

How a translation is named is a layout:

| Layout | Translations of `docs/guide.md` |
|---|---|
| `suffix` (the default) | `docs/guide.ja.md`, `docs/guide.zh-TW.md` |
| `dir` | `docs/ja/guide.md`, `docs/zh-TW/guide.md` |

`--translations` (or `$GH_MINI_TRANSLATIONS`) picks the layout, as the way a repository names its translations is its own.

A language is a two-letter code, such as `ja`, or one with a region, such as `zh-TW`. With `dir`, a directory named after a language, such as `id` or `it`, holds translations too, so pick `dir` only for a repository laid out that way.

## Templates

`--translations` takes templates too: the path of a translation from its original's directory, with `{name}` for the original's name and `{lang}` for the language.

| Template | Translation of `docs/guide.md` |
|---|---|
| `{name}_{lang}` | `docs/guide_ja.md` |
| `i18n/{lang}/{name}` | `docs/i18n/ja/guide.md` |

Separate layouts with commas to use several; the first that fits a file is taken.

## Turning translations off

`--translations off` leaves translations alone, as files of their own. Turning off Language switch in the settings does the same for you alone, in every gh-mini.
