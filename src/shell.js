import { spawn } from "node:child_process";

export function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\"'\"'")}'`;
}

export function formatCommand(command, args = []) {
  return [command, ...args].map(shellQuote).join(" ");
}

export function run(command, args, options = {}) {
  const {
    input,
    capture = true,
    env = process.env,
    cwd = process.cwd()
  } = options;

  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd,
      env,
      stdio: [input === undefined ? "ignore" : "pipe", capture ? "pipe" : "inherit", capture ? "pipe" : "inherit"]
    });
    let stdout = "";
    let stderr = "";

    if (capture) {
      child.stdout.setEncoding("utf8");
      child.stderr.setEncoding("utf8");
      child.stdout.on("data", (chunk) => { stdout += chunk; });
      child.stderr.on("data", (chunk) => { stderr += chunk; });
    }
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolve({ code, stdout, stderr });
        return;
      }
      reject(new Error(`${command} exited with ${code}${stderr ? `: ${stderr.trim()}` : ""}`));
    });
    if (input !== undefined) {
      child.stdin.end(input);
    }
  });
}

export function commandExists(command) {
  return run("sh", ["-c", `command -v ${shellQuote(command)}`]).then(
    () => true,
    () => false
  );
}
