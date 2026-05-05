# TOOLS

Tools are executable snippets for this vault.
Define each tool as a level-3 heading with one fenced `sh` block.

### vault-status
Show vault health.
```sh
bashcorrect vault status
```

### find-recent-markdown
List markdown files changed in the last day.
```sh
find . -type f -name '*.md' -mtime -1 | sort
```
