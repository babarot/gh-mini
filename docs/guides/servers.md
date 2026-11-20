# Several at once

Each gh-mini serves one directory, and several can run at once, one to a directory. A gh-mini takes the port given to `--port`, 6419 if none, or the next free one when it is taken.

`--list` prints the ones running, a port, the GitHub repository of its origin remote and a directory to a line, separated by tabs. A directory without a remote on GitHub has no repository:

```console
$ gh mini --list
6419		/Users/me/notes
6420	babarot/gh-mini	/Users/me/src/gh-mini
```

`--shutdown` stops those on the ports or serving the directories given:

```sh
gh mini --shutdown 6420
gh mini --shutdown .
```

Given none, it reads lines of `--list` from stdin and stops those on their ports, so one can be picked to stop:

```sh
gh mini --list | fzf | gh mini --shutdown
```

A gh-mini tells where it is in a file in `$XDG_STATE_HOME/gh-mini/servers` (`~/.local/state/gh-mini/servers` without it), removed when it stops. The file holds a token that `--shutdown` sends; a page in a browser cannot read it, so a page cannot stop a gh-mini. A gh-mini killed outright leaves its file behind; `--list` finds nothing answering there and removes it.
