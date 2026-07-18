// content/codeforces.js
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.action === "START_SCRAPING") {
    handleScrapeCF().then((payload) => {
      if (payload) {
        sendResponse({ success: true, data: payload });
      } else {
        sendResponse({
          success: false,
          error: "Failed to scrape Codeforces problem page.",
        });
      }
    });
  }
  return true; // Keep message channel open for async response
});

async function handleScrapeCF() {
  let payload = null;
  // 1. Check if a status modal popup is open on the screen
  const popup = document.querySelector(".source-popup");
  if (popup) {
    payload = await scrapeCFPopup(popup);
  }
  // 2. Check if we are on a dedicated submission page URL
  else if (window.location.href.includes("/submission/")) {
    payload = await scrapeCFSubmissionPage();
  }
  // 3. Otherwise, parse the regular problem statement page
  else {
    payload = scrapeCFProblemPage();
  }
  return payload;
}

// Parses standard problem statement page
function scrapeCFProblemPage() {
  try {
    const title =
      document.querySelector(".problem-statement .title")?.innerText ||
      "Codeforces Problem";

    // 💡 DYNAMIC LIMIT PARSING: Pull text blocks directly from the problem metadata header
    const timeLimitText =
      document.querySelector(".problem-statement .time-limit")?.innerText ||
      "2.0 seconds";
    const memLimitText =
      document.querySelector(".problem-statement .memory-limit")?.innerText ||
      "256 megabytes";

    // Extract numbers using simple Regex
    const timeNum = parseFloat(timeLimitText.match(/[\d.]+/)?.[0] || 2);
    const memNum = parseInt(memLimitText.match(/\d+/)?.[0] || 256);

    const timeLimitMs = timeNum * 1000;
    const memoryLimitMb = memNum;

    const inputBlocks = document.querySelectorAll(".sample-test .input pre");
    const outputBlocks = document.querySelectorAll(".sample-test .output pre");

    let tests = [];
    for (let i = 0; i < inputBlocks.length; i++) {
      tests.push({
        input: inputBlocks[i].innerText.trim() + "\n",
        expected: outputBlocks[i]
          ? outputBlocks[i].innerText.trim() + "\n"
          : "",
      });
    }

    if (tests.length === 0) return null;

    return {
      platform: "codeforces",
      title: title,
      type: "standard_io",
      data_structure: "standard",
      time_limit_ms: timeLimitMs,
      memory_limit_mb: memoryLimitMb,
      tests: tests,
    };
  } catch (err) {
    console.error("CF Problem page scraping failed:", err);
    return null;
  }
}

// A highly resilient parser for the status window modal popup
async function scrapeCFPopup(popupElement) {
  try {
    // Find any "Click to see the test case" reveal links in the popup
    const revealLinks = Array.from(popupElement.querySelectorAll("a")).filter(
      (a) => {
        const text = a.innerText.toLowerCase();
        return text.includes("click to see") || text.includes("reveal");
      },
    );

    let tests = [];

    if (revealLinks.length > 0) {
      for (let link of revealLinks) {
        const testCaseData = await fetchTestCaseData(link);
        if (testCaseData) {
          tests.push(testCaseData);
        }
      }
    }

    // Fallback: Parse the visible pre blocks if no reveal links are present (or failed)
    if (tests.length === 0) {
      const preBlocks = Array.from(popupElement.querySelectorAll("pre"));
      const testDataPresets = preBlocks.filter(
        (pre) => !pre.classList.contains("prettyprint"),
      );

      if (testDataPresets.length >= 2) {
        const inputData = testDataPresets[0].innerText.trim() + "\n";
        const expectedData = testDataPresets[1].innerText.trim() + "\n";
        tests.push({
          input: inputData,
          expected: expectedData,
        });
      }
    }

    if (tests.length === 0) return null;

    return {
      platform: "codeforces",
      title: "Codeforces Popup Debug Case",
      type: "standard_io",
      data_structure: "standard",
      time_limit_ms: 2000,
      memory_limit_mb: 256,
      tests: tests,
    };
  } catch (err) {
    console.error("CF Popup parsing failed:", err);
    return null;
  }
}

// Parses dedicated submission details page
async function scrapeCFSubmissionPage() {
  try {
    const problemTitle =
      document.querySelector(".file-input-view")?.innerText ||
      "CF Debug Problem";

    // Find any "Click to see the test case" reveal links
    const revealLinks = Array.from(document.querySelectorAll("a")).filter(
      (a) => {
        const text = a.innerText.toLowerCase();
        return text.includes("click to see") || text.includes("reveal");
      },
    );

    let tests = [];

    if (revealLinks.length > 0) {
      for (let link of revealLinks) {
        const testCaseData = await fetchTestCaseData(link);
        if (testCaseData) {
          tests.push(testCaseData);
        }
      }
    }

    // Fallback: parse direct input/answer views
    if (tests.length === 0) {
      const inputBlock = document.querySelector(".input-view pre");
      const expectedBlock = document.querySelector(".answer-view pre");

      if (inputBlock) {
        tests.push({
          input: inputBlock.innerText.trim() + "\n",
          expected: expectedBlock ? expectedBlock.innerText.trim() + "\n" : "",
        });
      }
    }

    if (tests.length === 0) return null;

    return {
      platform: "codeforces",
      title: problemTitle + " (Submission)",
      type: "standard_io",
      data_structure: "standard",
      time_limit_ms: 2000,
      memory_limit_mb: 256,
      tests: tests,
    };
  } catch (err) {
    console.error("CF Submission page parsing failed", err);
    return null;
  }
}

// Utility to resolve a reveal link (either via AJAX GET fetch or programmatic click observer)
async function fetchTestCaseData(link) {
  const href = link.getAttribute("href");

  // Method 1: Fetch via GET URL if valid (fast & silent)
  if (href && href !== "#" && !href.startsWith("javascript:")) {
    try {
      const res = await fetch(href);
      if (res.ok) {
        const html = await res.text();
        const parser = new DOMParser();
        const doc = parser.parseFromString(html, "text/html");

        const preBlocks = Array.from(doc.querySelectorAll("pre"));
        if (preBlocks.length >= 2) {
          return {
            input: preBlocks[0].innerText.trim() + "\n",
            expected: preBlocks[1].innerText.trim() + "\n",
          };
        }
      }
    } catch (err) {
      console.error("Failed to fetch test case from URL:", href, err);
    }
  }

  // Method 2: Click programmatically & poll the DOM for Facebox/Modal content
  return new Promise((resolve) => {
    link.click();
    let checkedCount = 0;

    const interval = setInterval(() => {
      checkedCount++;
      const modal = document.querySelector(
        '#facebox, .facebox, .modal, [id*="judge"], [class*="protocol"]',
      );
      if (modal) {
        const preBlocks = Array.from(modal.querySelectorAll("pre"));
        if (preBlocks.length >= 2) {
          clearInterval(interval);
          const input = preBlocks[0].innerText.trim() + "\n";
          const expected = preBlocks[1].innerText.trim() + "\n";

          // Attempt to close the popup automatically
          const closeBtn = modal.querySelector(
            ".close, a.close, .facebox-close",
          );
          if (closeBtn) {
            closeBtn.click();
          } else {
            const overlay = document.getElementById("facebox_overlay");
            if (overlay) overlay.click();
          }

          resolve({ input, expected });
          return;
        }
      }

      // Timeout after 3 seconds
      if (checkedCount > 30) {
        clearInterval(interval);
        resolve(null);
      }
    }, 100);
  });
}
