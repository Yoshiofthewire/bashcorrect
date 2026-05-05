# TOOLS

Tools are executable snippets for this vault.
Define each tool as a level-3 heading with one or more fenced code blocks.
Use a `sh` block for Unix/macOS and a `ps1` block for Windows — both are optional but at least one is required.

### vault-status
Show vault health.
```sh
bashcorrect vault status
```
```ps1
bashcorrect vault status
```

### find-recent-markdown
List markdown files changed in the last day.
```sh
find . -type f -name '*.md' -mtime -1 | sort
```
```ps1
Get-ChildItem -Recurse -Filter '*.md' | Where-Object { $_.LastWriteTime -gt (Get-Date).AddDays(-1) } | Sort-Object FullName | Select-Object -ExpandProperty FullName
```
