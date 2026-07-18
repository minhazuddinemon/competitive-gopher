// content/atcoder.js
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.action === "START_SCRAPING") {
    const payload = scrapeAtCoder();
    if (payload) {
      sendResponse({ success: true, data: payload });
    } else {
      sendResponse({ success: false, error: "Failed to scrape AtCoder problem page." });
    }
  }
  return true; // Keep channel open
});

function scrapeAtCoder() {
  try {
    const title =
      document.querySelector(".h2")?.innerText.trim() || "AtCoder Problem";

    // 💡 DYNAMIC LIMIT PARSING: Look at the text right above the problem statements
    const pageText = document.body.innerText;
    let timeLimitMs = 2000; // Safe code fallback
    let memoryLimitMb = 256;

    const timeMatch = pageText.match(/Time Limit:\s*([\d.]+)\s*sec/i);
    const memMatch = pageText.match(/Memory Limit:\s*(\d+)\s*MB/i);

    if (timeMatch) timeLimitMs = parseFloat(timeMatch[1]) * 1000;
    if (memMatch) memoryLimitMb = parseInt(memMatch[1]);

    const langEnZone = document.querySelector("span.lang-en");
    const searchRoot = langEnZone ? langEnZone : document;
    const sampleBlocks = searchRoot.querySelectorAll("div.part");

    let inputs = [];
    let outputs = [];

    sampleBlocks.forEach((block) => {
      const heading = block.querySelector("h3")?.innerText || "";
      const preBlock = block.querySelector("pre");

      if (preBlock) {
        if (heading.includes("Sample Input")) {
          inputs.push(preBlock.innerText.trim() + "\n");
        } else if (heading.includes("Sample Output")) {
          outputs.push(preBlock.innerText.trim() + "\n");
        }
      }
    });

    let tests = [];
    for (let i = 0; i < inputs.length; i++) {
      tests.push({ input: inputs[i], expected: outputs[i] || "" });
    }

    if (tests.length === 0) return null;

    return {
      platform: "atcoder",
      title: title,
      type: "standard_io",
      data_structure: "standard",
      time_limit_ms: timeLimitMs,
      memory_limit_mb: memoryLimitMb,
      tests: tests,
    };
  } catch (err) {
    console.error("AtCoder scraper failed:", err);
    return null;
  }
}
