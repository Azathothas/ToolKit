# skills/

Three skills for an agent that has to use this repository's products, and nothing
else to go on.

| skill | what it is for |
| --- | --- |
| [`wsl-toolkit/`](wsl-toolkit/SKILL.md) | building and operating a base: the configuration, `base ensure`, grants, `base exec`, and attacking the base rather than trusting a page |
| [`wsl-toolkit-agents/`](wsl-toolkit-agents/SKILL.md) | driving Muse Code, pi and omp through herdr, from Windows and from inside the base, including the model and the effort each one starts on |
| [`text-tool/`](text-tool/SKILL.md) | writing and editing files from a shell that would otherwise mangle the payload. It covers the safe channels, `--expect`, many files at once, and the `eol` mode |

⛔ **Each one stands alone.** It is handed to an agent with no other context, so it
repeats what it needs rather than pointing into this tree.

⛔ **Each one says how to get its tool and how to print its own reference.** That is
`wsl-toolkit man --no-pager`, `herdr agent --help` and `text-tool --help`. Each is
generated from what the executable really has, and a pasted list is wrong the day
after it is written.

⚠ **No skill hardcodes a `wsl-toolkit` or herdr flag list. `text-tool/` keeps two
short ones**, the common operations and the `eol` forms. They stay because the
check below reads every `text-tool` flag in them against the program's own usage
text, so they cannot drift silently. Every other flag is left to `text-tool --help`.

⭐ **These properties are checked, not promised.** `sh scripts/common/check.sh skills`
refuses a skill whose prose links out of this directory. In fenced lines only, it
refuses a `wsl-toolkit` command the generated manual does not document, a flag
that command's form does not list there, and a `text-tool` flag its usage text
does not document.

⛔ **The command check is the one a weak agent depends on.** A capable agent reads a
refusal and recovers. A weak one runs what the page said, gets exit 2, and reports
the task as impossible.

⚠ **They are not the technical reference.**
[`../tools/windows/wsl-toolkit/wsl-toolkit.md`](../tools/windows/wsl-toolkit/wsl-toolkit.md)
owns the tool's behaviour, and when a skill disagrees with it the skill is the defect.
