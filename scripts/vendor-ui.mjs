import { createRequire } from 'node:module';
import { writeFile } from 'node:fs/promises';
import path from 'node:path';

const require = createRequire(import.meta.url);
const lucide = require(process.argv[2] || 'lucide');
const names = ['Activity', 'Settings2', 'Sun', 'Moon', 'RefreshCw', 'Plus', 'Play', 'Pause', 'Clock3', 'Download', 'X', 'Search', 'Fingerprint', 'FlaskConical', 'History', 'Monitor', 'Check', 'Eye', 'EyeOff', 'ArrowUpRight', 'Power'];
const icons = Object.fromEntries(names.map(name => {
  if (!lucide.icons[name]) throw Error(`Missing Lucide icon: ${name}`);
  return [name, lucide.icons[name]];
}));
await writeFile(path.resolve(import.meta.dirname, '../internal/check/web/assets/icons.js'),
  '// Generated from Lucide (ISC); see third_party/lucide/LICENSE.\nexport const icons = ' + JSON.stringify(icons) + ';\n');
