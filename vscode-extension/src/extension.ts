import * as vscode from "vscode";
import * as path from "path";
import * as fs from "fs";

const TERMINAL_NAME = "Codecast";

// ---------------------------------------------------------------------------
// CodecastOutputChannel – parses codecast stdout for file-modification events
// ---------------------------------------------------------------------------

class CodecastOutputChannel {
  readonly channel: vscode.OutputChannel;
  private readonly fileModifiedPattern: RegExp;

  constructor() {
    this.channel = vscode.window.createOutputChannel("Codecast");
    // Matches common codecast output patterns like:
    //   ✏️  Modified auth.go
    //   Wrote file: auth.go
    //   Editing src/main.go
    this.fileModifiedPattern =
      /(?:Modified|Wrote file|Editing|Changed|Updated)\s+(.+\.\w+)/i;
  }

  appendLine(line: string): string | null {
    this.channel.appendLine(line);
    const match = this.fileModifiedPattern.exec(line);
    if (match && match[1]) {
      return match[1].trim();
    }
    return null;
  }

  show(): void {
    this.channel.show(true);
  }

  dispose(): void {
    this.channel.dispose();
  }
}

// ---------------------------------------------------------------------------
// CodecastFileWatcher – watches workspace files & offers diff view
// ---------------------------------------------------------------------------

interface FileSnapshot {
  uri: vscode.Uri;
  originalContent: string;
}

class CodecastFileWatcher {
  private watcher: vscode.FileSystemWatcher | undefined;
  private snapshots: Map<string, FileSnapshot> = new Map();
  private outputChannel: CodecastOutputChannel;
  private isActive = false;

  constructor(outputChannel: CodecastOutputChannel) {
    this.outputChannel = outputChannel;
  }

  start(): void {
    if (this.watcher) {
      return;
    }

    // Watch all files in the workspace
    this.watcher = vscode.workspace.createFileSystemWatcher("**/*");

    this.watcher.onDidChange(async (uri) => {
      if (!this.isActive) {
        return;
      }
      await this.handleFileChange(uri);
    });

    this.watcher.onDidCreate(async (uri) => {
      if (!this.isActive) {
        return;
      }
      // For newly created files we can't show a diff (no original), but we log it
      const fileName = path.basename(uri.fsPath);
      this.outputChannel.appendLine(`[Codecast] New file created: ${fileName}`);
    });
  }

  setActive(active: boolean): void {
    this.isActive = active;
  }

  /** Capture the current content of a file before codecast modifies it. */
  async captureSnapshot(uri: vscode.Uri): Promise<void> {
    try {
      const content = await fs.promises.readFile(uri.fsPath, "utf-8");
      this.snapshots.set(uri.fsPath, { uri, originalContent: content });
    } catch {
      // File may not exist yet – that's fine
    }
  }

  private async handleFileChange(uri: vscode.Uri): Promise<void> {
    const filePath = uri.fsPath;
    const fileName = path.basename(filePath);

    // Skip files outside the workspace
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(uri);
    if (!workspaceFolder) {
      return;
    }

    // Skip non-source files (node_modules, .git, etc.)
    if (
      filePath.includes("node_modules") ||
      filePath.includes(".git") ||
      filePath.includes("out/") ||
      filePath.includes("dist/")
    ) {
      return;
    }

    const config = vscode.workspace.getConfiguration("codecast");
    const autoShowDiff = config.get<boolean>("autoShowDiff", true);

    // If we have a snapshot, offer diff
    const snapshot = this.snapshots.get(filePath);
    if (snapshot) {
      this.outputChannel.appendLine(
        `[Codecast] File modified: ${fileName}`
      );

      if (autoShowDiff) {
        await this.showDiff(snapshot, fileName);
      } else {
        const choice = await vscode.window.showInformationMessage(
          `Codecast modified ${fileName}. View changes?`,
          "View Changes",
          "Dismiss"
        );
        if (choice === "View Changes") {
          await this.showDiff(snapshot, fileName);
        }
      }
      // Update snapshot so subsequent changes diff against latest
      await this.captureSnapshot(uri);
    } else {
      // No prior snapshot – capture now for future diffs
      this.outputChannel.appendLine(
        `[Codecast] File changed (no prior snapshot): ${fileName}`
      );
      await this.captureSnapshot(uri);
    }
  }

  async showDiff(snapshot: FileSnapshot, fileName: string): Promise<void> {
    const originalUri = vscode.Uri.parse(
      `codecast-original:${fileName}?${Date.now()}`
    );

    const provider = new (class implements vscode.TextDocumentContentProvider {
      private content: string;
      constructor(content: string) {
        this.content = content;
      }
      provideTextDocumentContent(): string {
        return this.content;
      }
    })(snapshot.originalContent);

    // Register a temporary provider for the virtual document
    const disposable = vscode.workspace.registerTextDocumentContentProvider(
      "codecast-original",
      provider
    );

    try {
      const title = `${fileName} (Original) ↔ ${fileName} (Modified)`;
      await vscode.commands.executeCommand(
        "vscode.diff",
        originalUri,
        snapshot.uri,
        title
      );
    } finally {
      // Dispose provider after a short delay so VS Code can read the content
      setTimeout(() => disposable.dispose(), 5000);
    }
  }

  /** Manually trigger diff for a file that has a snapshot. */
  async showDiffForFile(filePath: string): Promise<void> {
    const snapshot = this.snapshots.get(filePath);
    if (!snapshot) {
      vscode.window.showWarningMessage(
        "No original snapshot available for this file."
      );
      return;
    }
    const fileName = path.basename(filePath);
    await this.showDiff(snapshot, fileName);
  }

  dispose(): void {
    this.watcher?.dispose();
    this.watcher = undefined;
    this.snapshots.clear();
  }
}

// ---------------------------------------------------------------------------
// CodecastStatusBar – shows active/idle state, model, token count
// ---------------------------------------------------------------------------

class CodecastStatusBar {
  private statusItem: vscode.StatusBarItem;
  private _isActive = false;
  private _model = "";
  private _tokenCount = 0;

  constructor() {
    this.statusItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      50
    );
    this.statusItem.command = "codecast.start";
    this.render();
    this.statusItem.show();
  }

  setActive(active: boolean): void {
    this._isActive = active;
    this.render();
  }

  setModel(model: string): void {
    this._model = model;
    this.render();
  }

  setTokenCount(count: number): void {
    this._tokenCount = count;
    this.render();
  }

  private render(): void {
    if (this._isActive) {
      const parts = ["$(hubot) Codecast: Active"];
      if (this._model) {
        parts.push(this._model);
      }
      if (this._tokenCount > 0) {
        parts.push(`${this._tokenCount} tokens`);
      }
      this.statusItem.text = parts.join(" · ");
      this.statusItem.tooltip = "Codecast agent is running. Click to restart.";
      this.statusItem.backgroundColor = undefined;
    } else {
      this.statusItem.text = "$(hubot) Codecast: Idle";
      this.statusItem.tooltip = "Click to start Codecast.";
      this.statusItem.backgroundColor = undefined;
    }
  }

  dispose(): void {
    this.statusItem.dispose();
  }
}

// ---------------------------------------------------------------------------
// CodecastTerminal – manages the integrated terminal
// ---------------------------------------------------------------------------

class CodecastTerminal {
  private terminal: vscode.Terminal | undefined;
  private outputChannel: CodecastOutputChannel;
  private fileWatcher: CodecastFileWatcher;
  private statusBar: CodecastStatusBar;
  private disposables: vscode.Disposable[] = [];

  constructor(
    outputChannel: CodecastOutputChannel,
    fileWatcher: CodecastFileWatcher,
    statusBar: CodecastStatusBar
  ) {
    this.outputChannel = outputChannel;
    this.fileWatcher = fileWatcher;
    this.statusBar = statusBar;

    // Listen for terminal close to detect when codecast stops
    vscode.window.onDidCloseTerminal((closedTerminal) => {
      if (closedTerminal === this.terminal) {
        this.terminal = undefined;
        this.fileWatcher.setActive(false);
        this.statusBar.setActive(false);
      }
    }, this, this.disposables);
  }

  getOrCreate(): vscode.Terminal {
    if (
      !this.terminal ||
      this.terminal.exitStatus !== undefined
    ) {
      this.terminal = vscode.window.createTerminal(TERMINAL_NAME);

      // Start listening to the terminal output
      this.startListening();
    }
    return this.terminal;
  }

  private startListening(): void {
    if (!this.terminal) {
      return;
    }

    // We use a shell execution approach: launch codecast via a wrapper that
    // also pipes output to a temp file we can tail.  However, VS Code's
    // terminal API doesn't expose stdout directly.  Instead, we rely on
    // the file watcher to detect modifications and the output channel for
    // user-visible logging.
    //
    // For richer integration we could spawn codecast as a child process,
    // but that requires knowing the exact CLI invocation.  The file watcher
    // + status bar approach works regardless of how codecast is launched.
  }

  sendText(text: string): void {
    const terminal = this.getOrCreate();
    terminal.show();
    terminal.sendText(text);
    this.fileWatcher.setActive(true);
    this.statusBar.setActive(true);
  }

  stop(): void {
    if (this.terminal) {
      // Send Ctrl+C to stop the running process
      this.terminal.sendText("\x03");
      this.fileWatcher.setActive(false);
      this.statusBar.setActive(false);
      this.outputChannel.appendLine("[Codecast] Agent stopped by user.");
    }
  }

  dispose(): void {
    this.terminal?.dispose();
    this.terminal = undefined;
    for (const d of this.disposables) {
      d.dispose();
    }
    this.disposables = [];
  }
}

// ---------------------------------------------------------------------------
// Extension activate / deactivate
// ---------------------------------------------------------------------------

let codecastTerminal: CodecastTerminal;
let fileWatcher: CodecastFileWatcher;
let outputChannel: CodecastOutputChannel;
let statusBar: CodecastStatusBar;

export function activate(context: vscode.ExtensionContext): void {
  outputChannel = new CodecastOutputChannel();
  fileWatcher = new CodecastFileWatcher(outputChannel);
  statusBar = new CodecastStatusBar();
  codecastTerminal = new CodecastTerminal(outputChannel, fileWatcher, statusBar);

  // Start the file watcher immediately
  fileWatcher.start();

  const config = vscode.workspace.getConfiguration("codecast");

  // ---- codecast.start ----
  const startDisposable = vscode.commands.registerCommand(
    "codecast.start",
    async () => {
      const cliPath = config.get<string>("cliPath", "codecast");
      const model = config.get<string>("model", "");
      const command = model ? `${cliPath} --model ${model}` : cliPath;

      // Capture snapshots of all open files before starting
      for (const doc of vscode.workspace.textDocuments) {
        if (!doc.isUntitled && doc.uri.scheme === "file") {
          await fileWatcher.captureSnapshot(doc.uri);
        }
      }

      codecastTerminal.sendText(command);
      statusBar.setModel(model || "default");
      outputChannel.appendLine(`[Codecast] Started with command: ${command}`);
      outputChannel.show();
    }
  );

  // ---- codecast.sendToCodecast ----
  const sendDisposable = vscode.commands.registerCommand(
    "codecast.sendToCodecast",
    async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showWarningMessage("No active editor");
        return;
      }

      const selection = editor.selection;
      const cliPath = config.get<string>("cliPath", "codecast");

      // Capture snapshot before sending
      await fileWatcher.captureSnapshot(editor.document.uri);

      if (!selection.isEmpty) {
        const selectedText = editor.document.getText(selection);
        const escaped = selectedText.replace(/"/g, '\\"').replace(/\n/g, "\\n");
        codecastTerminal.sendText(`${cliPath} ask "${escaped}"`);
      } else {
        const filePath = editor.document.uri.fsPath;
        codecastTerminal.sendText(`${cliPath} ask --file "${filePath}"`);
      }
    }
  );

  // ---- codecast.reviewFile ----
  const reviewDisposable = vscode.commands.registerCommand(
    "codecast.reviewFile",
    async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showWarningMessage("No active editor");
        return;
      }

      const cliPath = config.get<string>("cliPath", "codecast");
      const filePath = editor.document.uri.fsPath;

      await fileWatcher.captureSnapshot(editor.document.uri);
      codecastTerminal.sendText(`${cliPath} review "${filePath}"`);
    }
  );

  // ---- codecast.showDiff ----
  const showDiffDisposable = vscode.commands.registerCommand(
    "codecast.showDiff",
    async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showWarningMessage("No active editor");
        return;
      }
      const filePath = editor.document.uri.fsPath;
      await fileWatcher.showDiffForFile(filePath);
    }
  );

  // ---- codecast.stopAgent ----
  const stopDisposable = vscode.commands.registerCommand(
    "codecast.stopAgent",
    () => {
      codecastTerminal.stop();
    }
  );

  // ---- Listen for configuration changes ----
  const configChangeDisposable = vscode.workspace.onDidChangeConfiguration(
    (e) => {
      if (e.affectsConfiguration("codecast.model")) {
        const newModel = vscode.workspace
          .getConfiguration("codecast")
          .get<string>("model", "");
        statusBar.setModel(newModel || "default");
      }
    }
  );

  // ---- Capture snapshots when documents open ----
  const docOpenDisposable = vscode.workspace.onDidOpenTextDocument(
    async (doc) => {
      if (!doc.isUntitled && doc.uri.scheme === "file") {
        await fileWatcher.captureSnapshot(doc.uri);
      }
    }
  );

  // ---- Parse terminal output for file-modification events ----
  // VS Code doesn't expose terminal stdout, but we can use a task-based
  // approach. For now, the file watcher handles detection. We also hook
  // into the onDidSave event to update token estimates.
  const saveDisposable = vscode.workspace.onDidSaveTextDocument((doc) => {
    if (!doc.isUntitled && doc.uri.scheme === "file") {
      const lineCount = doc.lineCount;
      // Rough token estimate (~4 chars per token)
      const estimatedTokens = Math.round(doc.getText().length / 4);
      statusBar.setTokenCount(estimatedTokens);
    }
  });

  context.subscriptions.push(
    startDisposable,
    sendDisposable,
    reviewDisposable,
    showDiffDisposable,
    stopDisposable,
    configChangeDisposable,
    docOpenDisposable,
    saveDisposable,
    outputChannel,
    fileWatcher,
    statusBar,
    codecastTerminal
  );
}

export function deactivate(): void {
  // Disposal is handled by context.subscriptions
}
