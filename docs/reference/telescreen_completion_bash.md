## telescreen completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(telescreen completion bash)

To load completions for every new session, execute once:

#### Linux:

	telescreen completion bash > /etc/bash_completion.d/telescreen

#### macOS:

	telescreen completion bash > $(brew --prefix)/etc/bash_completion.d/telescreen

You will need to start a new shell for this setup to take effect.


```
telescreen completion bash
```

### Options

```
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions
```

### SEE ALSO

* [telescreen completion](telescreen_completion.md)	 - Generate the autocompletion script for the specified shell

