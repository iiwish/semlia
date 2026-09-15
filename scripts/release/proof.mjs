import { createHash, sign, verify } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export function filesFor(version) {
  if (!/^v[0-9][0-9A-Za-z.+-]*$/.test(version)) throw new Error('Invalid release version');
  return ['linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64'].flatMap(platform => {
    const bundle = `semlia-${version}-${platform}`;
    return [`${bundle}/${bundle}.tar.gz`, `${bundle}/${bundle}.sbom.cdx.json`, `${bundle}/SHA256SUMS`];
  }).concat(`semlia-image-${version}/image.json`);
}

export function statement(root, version, commit, runURL) {
  if (!/^[a-f0-9]{40}$/.test(commit)) throw new Error('Invalid commit');
  if (!/^https:\/\/github.com\/iiwish\/semlia\/actions\/runs\/[0-9]+$/.test(runURL)) throw new Error('Invalid run URL');
  return { schema: 'semlia.release-proof/v1', repository: 'iiwish/semlia', version, commit, runURL,
    files: filesFor(version).map(path => ({ path, sha256: createHash('sha256').update(readFileSync(join(root, path))).digest('hex') })) };
}

export function verifyProof(root, bytes, signature, publicKey, version, commit) {
  if (!verify(null, bytes, publicKey, signature)) throw new Error('Invalid release signature');
  const proof = JSON.parse(bytes);
  if (proof.version !== version || proof.commit !== commit) throw new Error('Unexpected release identity');
  const expected = statement(root, version, commit, proof.runURL);
  if (JSON.stringify(proof) !== JSON.stringify(expected)) throw new Error('Release contents do not match proof');
  return proof;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [mode, root, version, commit, runURL] = process.argv.slice(2);
    const publicKey = readFileSync(new URL('./signing-public.pem', import.meta.url));
    if (mode === 'sign') {
      const bytes = Buffer.from(JSON.stringify(statement(root, version, commit, runURL)));
      const signature = sign(null, bytes, process.env.SEMLIA_RELEASE_SIGNING_KEY);
      verifyProof(root, bytes, signature, publicKey, version, commit);
      writeFileSync(join(root, 'release-proof.json'), bytes);
      writeFileSync(join(root, 'release-proof.sig'), signature);
    } else if (mode === 'verify') {
      verifyProof(root, readFileSync(join(root, 'release-proof.json')), readFileSync(join(root, 'release-proof.sig')), publicKey, version, commit);
    } else throw new Error('Expected sign or verify');
    console.log('Release proof verified');
  } catch {
    console.error('Release proof failed; check identity, files and signing key');
    process.exitCode = 1;
  }
}
