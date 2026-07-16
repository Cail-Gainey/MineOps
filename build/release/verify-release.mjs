import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { basename, join } from "node:path";
import process from "node:process";

const [directory, manifestName, sidecarName] = process.argv.slice(2);
if (!directory || !manifestName || !sidecarName) {
  throw new Error("usage: node verify-release.mjs <dir> <manifest> <sidecar>");
}
const manifestPath = join(directory, manifestName);
const manifestBytes = readFileSync(manifestPath);
const manifest = JSON.parse(manifestBytes);
const sidecar = JSON.parse(readFileSync(join(directory, sidecarName), "utf8"));
if (sidecar.filename !== basename(manifestPath) || sidecar.size !== manifestBytes.length) {
  throw new Error("manifest sidecar filename or size mismatch");
}
const digest = createHash("sha512").update(manifestBytes).digest();
if (digest.toString("base64") !== sidecar.digest) throw new Error("manifest digest mismatch");
if (sidecar.digestAlgo !== "sha512" || sidecar.signatureAlgo !== "ed25519ph") throw new Error("unsupported manifest signature algorithm");

const seen = new Set();
for (const artifact of manifest.artifacts ?? []) {
  const key = `${artifact.platform}/${artifact.arch}`;
  if (seen.has(key)) throw new Error(`duplicate updater artifact: ${key}`);
  seen.add(key);
  const file = join(directory, basename(artifact.url));
  if (!statSync(file).isFile()) throw new Error(`missing updater artifact: ${file}`);
}
if (seen.size === 0) throw new Error("manifest has no updater artifacts");

const checksumLines = readFileSync(join(directory, "SHA512SUMS"), "utf8").trim().split("\n");
const checksummed = new Set(checksumLines.map((line) => line.trim().split(/\s+/).at(-1)?.replace(/^\*/, "")));
for (const file of readdirSync(directory)) {
  if (file === "SHA512SUMS" && checksummed.has(file)) throw new Error("SHA512SUMS must not checksum itself");
  if (file !== "SHA512SUMS" && !checksummed.has(file)) throw new Error(`asset missing from SHA512SUMS: ${file}`);
}
