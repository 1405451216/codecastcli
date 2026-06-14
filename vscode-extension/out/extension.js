"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = __importStar(require("vscode"));
const path = __importStar(require("path"));
const fs = __importStar(require("fs"));
const TERMINAL_NAME = "Codecast";
// ---------------------------------------------------------------------------
// CodecastOutputChannel – parses codecast stdout for file-modification events
// ---------------------------------------------------------------------------
class CodecastOutputChannel {
    constructor() {
        this.channel = vscode.window.createOutputChannel("Codecast");
        // Matches common codecast output patterns like:
        //   ✏️  Modified auth.go
        //   Wrote file: auth.go
        //   Editing src/main.go
        this.fileModifiedPattern =
            /(?:Modified|Wrote file|Editing|Changed|Updated)\s+(.+\.\w+)/i;
    }
    appendLine(line) {
        this.channel.appendLine(line);
        const match = this.fileModifiedPattern.exec(line);
        if (match && match[1]) {
            return match[1].trim();
        }
        return null;
    }
    show() {
        this.channel.show(true);
    }
    dispose() {
        this.channel.dispose();
    }
}
class CodecastFileWatcher {
    constructor(outputChannel) {
        this.snapshots = new Map();
        this.isActive = false;
        this.outputChannel = outputChannel;
    }
    start() {
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
    setActive(active) {
        this.isActive = active;
    }
    /** Capture the current content of a file before codecast modifies it. */
    async captureSnapshot(uri) {
        try {
            const content = await fs.promises.readFile(uri.fsPath, "utf-8");
            this.snapshots.set(uri.fsPath, { uri, originalContent: content });
        }
        catch {
            // File may not exist yet – that's fine
        }
    }
    async handleFileChange(uri) {
        const filePath = uri.fsPath;
        const fileName = path.basename(filePath);
        // Skip files outside the workspace
        const workspaceFolder = vscode.workspace.getWorkspaceFolder(uri);
        if (!workspaceFolder) {
            return;
        }
        // Skip non-source files (node_modules, .git, etc.)
        if (filePath.includes("node_modules") ||
            filePath.includes(".git") ||
            filePath.includes("out/") ||
            filePath.includes("dist/")) {
            return;
        }
        const config = vscode.workspace.getConfiguration("codecast");
        const autoShowDiff = config.get("autoShowDiff", true);
        // If we have a snapshot, offer diff
        const snapshot = this.snapshots.get(filePath);
        if (snapshot) {
            this.outputChannel.appendLine(`[Codecast] File modified: ${fileName}`);
            if (autoShowDiff) {
                await this.showDiff(snapshot, fileName);
            }
            else {
                const choice = await vscode.window.showInformationMessage(`Codecast modified ${fileName}. View changes?`, "View Changes", "Dismiss");
                if (choice === "View Changes") {
                    await this.showDiff(snapshot, fileName);
                }
            }
            // Update snapshot so subsequent changes diff against latest
            await this.captureSnapshot(uri);
        }
        else {
            // No prior snapshot – capture now for future diffs
            this.outputChannel.appendLine(`[Codecast] File changed (no prior snapshot): ${fileName}`);
            await this.captureSnapshot(uri);
        }
    }
    async showDiff(snapshot, fileName) {
        const originalUri = vscode.Uri.parse(`codecast-original:${fileName}?${Date.now()}`);
        const provider = new (class {
            constructor(content) {
                this.content = content;
            }
            provideTextDocumentContent() {
                return this.content;
            }
        })(snapshot.originalContent);
        // Register a temporary provider for the virtual document
        const disposable = vscode.workspace.registerTextDocumentContentProvider("codecast-original", provider);
        try {
            const title = `${fileName} (Original) ↔ ${fileName} (Modified)`;
            await vscode.commands.executeCommand("vscode.diff", originalUri, snapshot.uri, title);
        }
        finally {
            // Dispose provider after a short delay so VS Code can read the content
            setTimeout(() => disposable.dispose(), 5000);
        }
    }
    /** Manually trigger diff for a file that has a snapshot. */
    async showDiffForFile(filePath) {
        const snapshot = this.snapshots.get(filePath);
        if (!snapshot) {
            vscode.window.showWarningMessage("No original snapshot available for this file.");
            return;
        }
        const fileName = path.basename(filePath);
        await this.showDiff(snapshot, fileName);
    }
    dispose() {
        this.watcher?.dispose();
        this.watcher = undefined;
        this.snapshots.clear();
    }
}
// ---------------------------------------------------------------------------
// CodecastStatusBar – shows active/idle state, model, token count
// ---------------------------------------------------------------------------
class CodecastStatusBar {
    constructor() {
        this._isActive = false;
        this._model = "";
        this._tokenCount = 0;
        this.statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
        this.statusItem.command = "codecast.start";
        this.render();
        this.statusItem.show();
    }
    setActive(active) {
        this._isActive = active;
        this.render();
    }
    setModel(model) {
        this._model = model;
        this.render();
    }
    setTokenCount(count) {
        this._tokenCount = count;
        this.render();
    }
    render() {
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
        }
        else {
            this.statusItem.text = "$(hubot) Codecast: Idle";
            this.statusItem.tooltip = "Click to start Codecast.";
            this.statusItem.backgroundColor = undefined;
        }
    }
    dispose() {
        this.statusItem.dispose();
    }
}
// ---------------------------------------------------------------------------
// CodecastTerminal – manages the integrated terminal
// ---------------------------------------------------------------------------
class CodecastTerminal {
    constructor(outputChannel, fileWatcher, statusBar) {
        this.disposables = [];
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
    getOrCreate() {
        if (!this.terminal ||
            this.terminal.exitStatus !== undefined) {
            this.terminal = vscode.window.createTerminal(TERMINAL_NAME);
            // Start listening to the terminal output
            this.startListening();
        }
        return this.terminal;
    }
    startListening() {
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
    sendText(text) {
        const terminal = this.getOrCreate();
        terminal.show();
        terminal.sendText(text);
        this.fileWatcher.setActive(true);
        this.statusBar.setActive(true);
    }
    stop() {
        if (this.terminal) {
            // Send Ctrl+C to stop the running process
            this.terminal.sendText("\x03");
            this.fileWatcher.setActive(false);
            this.statusBar.setActive(false);
            this.outputChannel.appendLine("[Codecast] Agent stopped by user.");
        }
    }
    dispose() {
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
let codecastTerminal;
let fileWatcher;
let outputChannel;
let statusBar;
function activate(context) {
    outputChannel = new CodecastOutputChannel();
    fileWatcher = new CodecastFileWatcher(outputChannel);
    statusBar = new CodecastStatusBar();
    codecastTerminal = new CodecastTerminal(outputChannel, fileWatcher, statusBar);
    // Start the file watcher immediately
    fileWatcher.start();
    const config = vscode.workspace.getConfiguration("codecast");
    // ---- codecast.start ----
    const startDisposable = vscode.commands.registerCommand("codecast.start", async () => {
        const cliPath = config.get("cliPath", "codecast");
        const model = config.get("model", "");
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
    });
    // ---- codecast.sendToCodecast ----
    const sendDisposable = vscode.commands.registerCommand("codecast.sendToCodecast", async () => {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
            vscode.window.showWarningMessage("No active editor");
            return;
        }
        const selection = editor.selection;
        const cliPath = config.get("cliPath", "codecast");
        // Capture snapshot before sending
        await fileWatcher.captureSnapshot(editor.document.uri);
        if (!selection.isEmpty) {
            const selectedText = editor.document.getText(selection);
            const escaped = selectedText.replace(/"/g, '\\"').replace(/\n/g, "\\n");
            codecastTerminal.sendText(`${cliPath} ask "${escaped}"`);
        }
        else {
            const filePath = editor.document.uri.fsPath;
            codecastTerminal.sendText(`${cliPath} ask --file "${filePath}"`);
        }
    });
    // ---- codecast.reviewFile ----
    const reviewDisposable = vscode.commands.registerCommand("codecast.reviewFile", async () => {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
            vscode.window.showWarningMessage("No active editor");
            return;
        }
        const cliPath = config.get("cliPath", "codecast");
        const filePath = editor.document.uri.fsPath;
        await fileWatcher.captureSnapshot(editor.document.uri);
        codecastTerminal.sendText(`${cliPath} review "${filePath}"`);
    });
    // ---- codecast.showDiff ----
    const showDiffDisposable = vscode.commands.registerCommand("codecast.showDiff", async () => {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
            vscode.window.showWarningMessage("No active editor");
            return;
        }
        const filePath = editor.document.uri.fsPath;
        await fileWatcher.showDiffForFile(filePath);
    });
    // ---- codecast.stopAgent ----
    const stopDisposable = vscode.commands.registerCommand("codecast.stopAgent", () => {
        codecastTerminal.stop();
    });
    // ---- Listen for configuration changes ----
    const configChangeDisposable = vscode.workspace.onDidChangeConfiguration((e) => {
        if (e.affectsConfiguration("codecast.model")) {
            const newModel = vscode.workspace
                .getConfiguration("codecast")
                .get("model", "");
            statusBar.setModel(newModel || "default");
        }
    });
    // ---- Capture snapshots when documents open ----
    const docOpenDisposable = vscode.workspace.onDidOpenTextDocument(async (doc) => {
        if (!doc.isUntitled && doc.uri.scheme === "file") {
            await fileWatcher.captureSnapshot(doc.uri);
        }
    });
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
    context.subscriptions.push(startDisposable, sendDisposable, reviewDisposable, showDiffDisposable, stopDisposable, configChangeDisposable, docOpenDisposable, saveDisposable, outputChannel, fileWatcher, statusBar, codecastTerminal);
}
function deactivate() {
    // Disposal is handled by context.subscriptions
}
//# sourceMappingURL=extension.js.map