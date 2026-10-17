// Verify a supported development-tool fallback across the VM boundary.
const fs = require('node:fs');
const path = '/workspace/polling-target';
const timer = setTimeout(() => {
  fs.unwatchFile(path);
  process.exitCode = 1;
}, 10000);
fs.watchFile(path, { interval: 50 }, () => {
  const text = fs.readFileSync(path, 'utf8');
  if (text === 'guest-positive-control') {
    fs.writeFileSync('/control/poll-ready', 'ready');
  } else if (text === 'host-poll-change') {
    fs.writeFileSync('/control/poll-observed.tmp', text);
    fs.renameSync('/control/poll-observed.tmp', '/control/poll-observed');
    fs.unwatchFile(path);
    clearTimeout(timer);
  }
});
// Wait for an actual callback before the host changes the file.
setTimeout(() => fs.writeFileSync(path, 'guest-positive-control'), 100);
