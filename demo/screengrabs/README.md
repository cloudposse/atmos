## Usage

Add all commands to `commands.txt` (use `#` to disable generation of the command)

Build all colorized HTML snippets of command output by running:

```shell
make all
```

Then install those into `../../website/static/screengrabs' by running

```shell
make install
```

All the files in `website/static/screengrabs` should be committed.


Use these screengrabs then inside of any MDX documentation file, add something like this:

```
import Screengrab from '@site/src/components/Screengrab'

<Screengrab title="atmos help" slug="atmos--help" />
```

> [!TIP]
> If you have multiple screengrabs inside the same `.mdx` file, make sure to name each component something unique.
