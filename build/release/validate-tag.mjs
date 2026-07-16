import process from "node:process";

const tag = process.argv[2] ?? "";
const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/.exec(tag);
if (!match) {
  throw new Error(`Release tag must be v<semver>, received: ${tag || "<empty>"}`);
}

const version = tag.slice(1);
const channel = match[4] ? "beta" : "stable";
const prerelease = channel === "beta";
for (const [key, value] of Object.entries({ version, channel, prerelease })) {
  process.stdout.write(`${key}=${value}\n`);
}
