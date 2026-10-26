import { spawn } from "node:child_process";

const command = process.platform === "win32" ? "docusaurus.cmd" : "docusaurus";
const port = process.env.CONDUCTOR_PORT || "3000";
const env = { ...process.env };

if (!env.BROWSER && process.platform !== "win32") {
  env.BROWSER = "open";
}

const child = spawn(command, ["start", "--port", port], {
  env,
  stdio: "inherit",
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }

  process.exit(code ?? 1);
});

child.on("error", (error) => {
  console.error(error.message);
  process.exit(1);
});
