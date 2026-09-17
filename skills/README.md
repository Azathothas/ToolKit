# skills/

Two skills for an agent that has to use this repository's one product, and nothing
else to go on.

| skill | what it is for |
| --- | --- |
| [`wsl-toolkit/`](wsl-toolkit/SKILL.md) | building and operating a base: the configuration, `base ensure`, grants, `base exec`, and attacking the base rather than trusting a page |
| [`wsl-toolkit-agents/`](wsl-toolkit-agents/SKILL.md) | driving Muse Code, pi and omp through herdr, from Windows and from inside the base, including the model and the effort each one starts on |

⛔ **Each one stands alone.** It is handed to an agent with no other context, so it
repeats what it needs rather than pointing into this tree.

⛔ **Neither hardcodes a flag list.** Both say how to install or self-update the tool
and how to print the manual, because `wsl-toolkit man --no-pager` is generated from the
commands the executable really has and a pasted list is wrong the day after it is
written.

⚠ **They are not the technical reference.**
[`../tools/windows/wsl-toolkit/wsl-toolkit.md`](../tools/windows/wsl-toolkit/wsl-toolkit.md)
owns the tool's behaviour, and when a skill disagrees with it the skill is the defect.
