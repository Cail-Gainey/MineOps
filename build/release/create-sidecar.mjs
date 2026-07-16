import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import process from "node:process";

const [manifest, privateKey, output] = process.argv.slice(2);
if (!manifest || !privateKey || !output) {
  throw new Error("usage: node create-sidecar.mjs <manifest> <private-key> <output>");
}

const signed = JSON.parse(execFileSync("wails3", ["updater", "sign", "-key", privateKey, manifest], { encoding: "utf8" }));
if (!Array.isArray(signed) || signed.length !== 1) {
  throw new Error("wails3 updater sign returned an unexpected result");
}
const entry = signed[0];
const bytes = readFileSync(manifest);
if (entry.size !== bytes.length || entry.filename !== manifest.split(/[\\/]/).at(-1)) {
  throw new Error("manifest signature metadata does not match the signed file");
}
writeFileSync(output, `${JSON.stringify(entry, null, 2)}\n`);
