# HTML previews

An HTML file's Preview shows it as a browser does, with its styles, images and scripts, module scripts and `fetch` included. Turn on HTML preview in the settings to open HTML files that way; the Preview / Code switch works either way.

## How previews are served

The preview comes from a second server on its own port, so the file's scripts run in an origin of their own and cannot reach gh-mini's pages. That server answers only the gh-mini pages that show a preview, through a cookie a page on another site cannot send. A path in the file starting with `/` resolves against the directory gh-mini serves.

The port is a free one, printed at start; set it with `--preview-port`. Through an SSH tunnel, forward the preview port too.

## What a preview's scripts can do

What a previewed file's scripts can still do, as with `python -m http.server`:

- read the files under the directory gh-mini serves, and send them anywhere
- read and write the cookies of the host, those of other servers on `localhost` included, but not HttpOnly ones

So preview only files you trust.

Opening a file raw does not run its scripts, whatever its type.
