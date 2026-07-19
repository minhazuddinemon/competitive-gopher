// content/cses.js
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.action === "START_SCRAPING") {
    scrapeCSES().then((payload) => {
      if (payload) {
        sendResponse({ success: true, data: payload });
      } else {
        sendResponse({
          success: false,
          error: "Failed to scrape CSES problem page.",
        });
      }
    });
  }
  return true; // Keep channel open for async response
});

async function scrapeCSES() {
  try {
    // Problem ID from the URL: https://cses.fi/problemset/task/1068(/)
    const idMatch = window.location.pathname.match(/\/task\/(\d+)/);
    if (!idMatch) {
      console.error("CSES scraping failed: no problem ID found in URL.");
      return null;
    }
    const problemId = idMatch[1];

    const title =
      document.querySelector("h1")?.innerText.trim() || "CSES Problem";

    // Time/Memory limit — CSES prints these as plain <li> text, e.g.
    // "Time limit: 1.00 s" / "Memory limit: 512 MB". Scan all <li>
    // elements rather than assuming a specific container, since the exact
    // wrapping <ul> isn't uniquely classed.
    let timeLimitMs = 1000;
    let memoryLimitMb = 512;

    document.querySelectorAll("li").forEach((li) => {
      const text = li.innerText;
      if (/time limit/i.test(text)) {
        const secs = parseFloat(text.match(/[\d.]+/)?.[0]);
        if (!isNaN(secs)) timeLimitMs = Math.round(secs * 1000);
      } else if (/memory limit/i.test(text)) {
        const mb = parseInt(text.match(/\d+/)?.[0], 10);
        if (!isNaN(mb)) memoryLimitMb = mb;
      }
    });

    // Example test case(s) — scoped specifically to the section under the
    // "Example"/"Examples" <h2>, NOT the whole page. The page also has
    // generic "Input"/"Output" <h2> sections describing the I/O format
    // (no test data), so grepping the whole page's text for "Input:" would
    // risk picking those up instead.
    const tests = extractExampleTests();

    if (tests.length === 0) {
      console.error("CSES scraping failed: no example test cases found.");
      return null;
    }

    return {
      platform: "cses",
      title: title,
      type: "standard_io",
      data_structure: "standard",
      time_limit_ms: timeLimitMs,
      memory_limit_mb: memoryLimitMb,
      tests: tests,
      problem_id: problemId,
    };
  } catch (err) {
    console.error("CSES scraping failed:", err);
    return null;
  }
}

// extractExampleTests walks the DOM starting at the "Example"/"Examples"
// heading, and collects <pre> blocks up to the next heading (or end of
// content). The heading level isn't assumed to be h2 -- CSES may render
// title and section headers (Input/Output/Constraints/Example) at the
// same level, so all of h1-h4 are searched and the walk stops at the
// next heading of ANY level, not specifically h2.
//
// Each <pre> is labeled by the text of its own preceding <p> ("Input:" /
// "Output:") rather than assumed by strict alternating position, so a
// page with an unexpected extra paragraph in between doesn't silently
// mis-pair input/output.
function extractExampleTests() {
  const HEADING_TAGS = ["H1", "H2", "H3", "H4"];
  const headings = Array.from(document.querySelectorAll("h1,h2,h3,h4"));
  const exampleHeading = headings.find((h) =>
    /^examples?$/i.test(h.innerText.trim()),
  );
  if (!exampleHeading) {
    console.error(
      "CSES scraping: no 'Example'/'Examples' heading found among h1-h4.",
    );
    return [];
  }

  const tests = [];
  let pendingLabel = null; // "input" | "output" | null
  let pendingInput = null;

  let node = exampleHeading.nextElementSibling;
  while (node && !HEADING_TAGS.includes(node.tagName)) {
    const tag = node.tagName;

    if (tag === "P") {
      const text = node.innerText.toLowerCase();
      if (text.includes("input")) pendingLabel = "input";
      else if (text.includes("output")) pendingLabel = "output";
    } else if (tag === "PRE") {
      const content = node.innerText;
      if (pendingLabel === "input") {
        pendingInput = content;
      } else if (pendingLabel === "output" && pendingInput !== null) {
        tests.push({
          input: pendingInput.trim() + "\n",
          expected: content.trim() + "\n",
        });
        pendingInput = null;
      }
      pendingLabel = null;
    }

    node = node.nextElementSibling;
  }

  if (tests.length === 0) {
    console.error(
      "CSES scraping: found the Example heading but no Input/Output <pre> pairs under it.",
    );
  }

  return tests;
}
