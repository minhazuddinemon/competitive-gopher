// background.js
// Competitive Gopher background service worker.
// Action triggers popup.html directly, so clicked listener is handled there.
chrome.runtime.onInstalled.addListener(() => {
  console.log("Competitive Gopher Extension Installed.");
});
