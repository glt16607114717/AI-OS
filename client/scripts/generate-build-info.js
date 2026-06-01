const fs = require('fs');
const path = require('path');

const pkg = require('../package.json');

const buildInfo = {
  build_time: new Date().toISOString().slice(0, 19),
  version: pkg.version
};

const outputDir = path.join(__dirname, '..', 'backend', 'python');
if (!fs.existsSync(outputDir)) {
  fs.mkdirSync(outputDir, { recursive: true });
}

fs.writeFileSync(
  path.join(outputDir, 'build_info.json'),
  JSON.stringify(buildInfo, null, 2) + '\n'
);

console.log('Build info:', JSON.stringify(buildInfo));

const scriptsSourceDir = path.join(__dirname, '..', 'resources', 'backend', 'python');
const scriptsDestDir = outputDir;

const scriptsToCopy = ['download_python.ps1', 'install_deps.ps1'];

for (const script of scriptsToCopy) {
  const src = path.join(scriptsSourceDir, script);
  const dst = path.join(scriptsDestDir, script);
  if (fs.existsSync(src)) {
    fs.copyFileSync(src, dst);
    console.log('Copied:', script);
  } else {
    console.warn('Warning: script not found:', src);
  }
}
