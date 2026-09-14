import fs from 'node:fs';
import { spawnSync } from 'node:child_process';

const [name, command, ...args] = process.argv.slice(2);
const clearedEnvironment = ['SEMLIA_INGESTION_TEST_DATABASE_URL', 'SEMLIA_EXECUTION_CRASH_HELPER', 'SEMLIA_EXECUTION_TEST_DB', 'SEMLIA_DATABASE_URL'];
const env = { ...process.env };
for (const key of clearedEnvironment) delete env[key];
const start = new Date().toISOString();
const result = spawnSync(command, args, { env, encoding: 'utf8', maxBuffer: 100 * 1024 * 1024 });
const record = { command: [command, ...args], clearedEnvironment, start, end: new Date().toISOString(), exitStatus: result.status, signal: result.signal, stdout: result.stdout, stderr: result.stderr, error: result.error?.message };
fs.writeFileSync(new URL(`${name}.json`, import.meta.url), JSON.stringify(record, null, 2), { flag: 'wx' });
process.stdout.write(result.stdout || '');
process.stderr.write(result.stderr || '');
process.exit(result.status ?? 1);
