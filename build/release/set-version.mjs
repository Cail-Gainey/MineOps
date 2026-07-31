import { readFileSync, writeFileSync } from "node:fs";
import process from "node:process";

const version = process.argv[2] ?? "";
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`invalid release version: ${version || "<empty>"}`);
}

const replacements = [
  ["Taskfile.yml", /(APP_VERSION:\s+")[^"]+(")/, `$1${version}$2`],
  ["internal/global/constants/constants.go", /(ApplicationVersion\s+=\s+")[^"]+(")/, `$1${version}$2`],
  ["internal/global/constants/constants.go", /(ApplicationUserAgent\s+=\s+"MineOps\/)[^"]+(")/, `$1${version}$2`],
  ["frontend/package.json", /("version"\s*:\s*")[^"]+(")/, `$1${version}$2`],
  ["build/config.yml", /(version:\s+")[^"]+("\s+# The application version)/, `$1${version}$2`],
  ["build/linux/nfpm/nfpm.yaml", /^(version:\s+")[^"]+("\s*)$/m, `$1${version}$2`],
  ["build/windows/info.json", /("(?:file_version|ProductVersion)"\s*:\s*")[^"]+("?)/g, `$1${version}$2`],
  ["build/windows/nsis/project.nsi", /(INFO_PRODUCTVERSION\s+")[^"]+("?)/g, `$1${version}$2`],
  ["build/windows/nsis/wails_tools.nsh", /(INFO_PRODUCTVERSION\s+")[^"]+("?)/g, `$1${version}$2`],
  ["build/windows/wails.exe.manifest", /(<assemblyIdentity\s+type="win32"\s+name="com\.gainey\.mineops"\s+version=")[^"]+(")/, `$1${version}.0$2`],
  ["build/darwin/Info.plist", /(<key>CFBundle(?:ShortVersionString|Version)<\/key>\s*<string>)[^<]+(<\/string>)/g, `$1${version}$2`],
  ["build/darwin/Info.dev.plist", /(<key>CFBundle(?:ShortVersionString|Version)<\/key>\s*<string>)[^<]+(<\/string>)/g, `$1${version}$2`],
];

for (const [path, pattern, replacement] of replacements) {
  const before = readFileSync(path, "utf8");
  const after = before.replace(pattern, replacement);
  if (after === before && !before.includes(version)) throw new Error(`version pattern not found in ${path}`);
  writeFileSync(path, after);
}
