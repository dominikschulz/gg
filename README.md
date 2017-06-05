# gg - Go Git Grep

Go Git Grep is a small search and replace tool. It works like grep, but is
able to replace in files, recursively.

## Example

    gg '^\s+(.+)\.Log\("level", "(.[^"]+)", ' $GOPATH/src/github.com/dominikschulz/ --exclude vendor/ --workers 20 --replace 'level.${2}(${1}).Log(' --eat-my-data
    gg 'level.error\(' $GOPATH/src/github.com/dominikschulz/ --exclude vendor/ --workers 20 --replace 'level.Error(' --eat-my-data --post-proc 'goimports -w'

## FAQ

* Similar tools: [grep](https://www.gnu.org/software/grep/), [ack](https://beyondgrep.com/), [the silver searcher](https://github.com/ggreer/the_silver_searcher), [ripgrep](https://github.com/BurntSushi/ripgrep)
* Should I use this tool? - Probably not. Use [ripgrep](https://github.com/BurntSushi/ripgrep) or your fancy IDE.
* Will this tool destroy my data? - Probably yes. At least if you play around with `--eat-my-data` and `--who-needs-backups`
* Seriously? Yes. You should know what you do.
* Why did you write it then? Because I wanted recursive search and replace, with git support, on Unix platforms without overloaded IDEs.
