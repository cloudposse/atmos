## Usage

Add all commands to `commands.txt` (use `#` to disable generation of the command)

Build all colorized HTML snippets of command output by running:

```shell
atmos screengrabs build --all
```

Build a subset by passing text from the command or artifact slug:

```shell
atmos screengrabs build about
```

Then install those into `../../website/src/components/Screengrabs` by running

```shell
atmos screengrabs install
```

All the files in `website/src/components/Screengrabs` should be committed.


Use these screengrabs then inside of any MDX documentation file, add something like this:

```
import Screengrab from '@site/src/components/Screengrab'

<Screengrab title="atmos help" slug="atmos--help" />
```

> [!TIP]
> If you have multiple screengrabs inside the same `.mdx` file, make sure to name each component something unique.
