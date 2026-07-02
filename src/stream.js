import { spawn } from "node:child_process";
import { assertSshAlias } from "./safety.js";

export function streamSshToSsh({ source, sourceCommand, target, targetCommand }) {
  assertSshAlias(source, "Source SSH alias");
  assertSshAlias(target, "Target SSH alias");

  return new Promise((resolve, reject) => {
    const sourceProcess = spawn("ssh", ["-o", "BatchMode=yes", "--", source, sourceCommand], {
      stdio: ["ignore", "pipe", "pipe"]
    });
    const targetProcess = spawn("ssh", ["-o", "BatchMode=yes", "--", target, targetCommand], {
      stdio: ["pipe", "inherit", "pipe"]
    });
    sourceProcess.stdout.pipe(targetProcess.stdin);

    let sourceError = "";
    let targetError = "";
    sourceProcess.stderr.setEncoding("utf8");
    targetProcess.stderr.setEncoding("utf8");
    sourceProcess.stderr.on("data", (chunk) => { sourceError += chunk; });
    targetProcess.stderr.on("data", (chunk) => { targetError += chunk; });

    let sourceCode;
    let targetCode;
    const finish = () => {
      if (sourceCode === undefined || targetCode === undefined) return;
      if (sourceCode === 0 && targetCode === 0) {
        resolve();
      } else {
        reject(new Error(`Stream failed (source=${sourceCode}, target=${targetCode}): ${sourceError}${targetError}`.trim()));
      }
    };
    sourceProcess.on("error", reject);
    targetProcess.on("error", reject);
    sourceProcess.on("close", (code) => { sourceCode = code; finish(); });
    targetProcess.on("close", (code) => { targetCode = code; finish(); });
  });
}
