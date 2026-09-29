import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageName = "@dunkymole/grpc-bridge";
const lockEntryPath = "node_modules/@dunkymole/grpc-bridge";
const comparedMetadata = [
  "dependencies",
  "optionalDependencies",
  "peerDependencies",
  "peerDependenciesMeta",
  "engines",
  "os",
  "cpu",
];

function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value).sort().map((key) => [key, canonical(value[key])]),
    );
  }
  return value;
}

export function bindPackedBridgeToLock({ lockPath, manifestPath, tarballPath }) {
  const lock = JSON.parse(readFileSync(lockPath, "utf8"));
  const entry = lock.packages?.[lockEntryPath];
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (!entry || manifest.name !== packageName || manifest.version !== entry.version) {
    throw new Error("bridge package name/version differs from the consumer lock; update the consumer lock before using this ref");
  }
  for (const key of comparedMetadata) {
    if (JSON.stringify(canonical(manifest[key] ?? {})) !== JSON.stringify(canonical(entry[key] ?? {}))) {
      throw new Error(`bridge package ${key} differs from the consumer lock; update the consumer lock before using this ref`);
    }
  }
  const tarball = readFileSync(tarballPath);
  entry.integrity = `sha512-${createHash("sha512").update(tarball).digest("base64")}`;
  writeFileSync(lockPath, `${JSON.stringify(lock, null, 2)}\n`);
  return entry.integrity;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [lockPath = "package-lock.json", manifestPath = "grpc-bridge-package.json", tarballPath = "grpc-bridge.tgz"] = process.argv.slice(2);
  bindPackedBridgeToLock({ lockPath, manifestPath, tarballPath });
}
