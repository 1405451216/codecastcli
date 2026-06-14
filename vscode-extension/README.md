# Codecast AI

AI-powered coding assistant embedded in VS Code.

## Usage

1. Open the Command Palette (`Ctrl+Shift+P`)
2. Run **Codecast: Start** to launch a Codecast terminal
3. Select text in the editor, right-click, and choose **Send to Codecast** to ask about the selection
4. Run **Codecast: Review File** to get an AI review of the current file

## Configuration

- `codecast.cliPath` — Path to the codecast CLI executable (default: `codecast`)
- `codecast.model` — Default model to use (leave empty for CLI default)

## Development

```bash
npm install
npm run compile
```

Press F5 in VS Code to launch the extension in a new Extension Development Host window.
