// popup.js

let problemData = null;
let clipboardDebounceTimer = null;

// Platform assets mapping
const PLATFORM_ICONS = {
  codeforces: "icons/codeforces.png",
  atcoder: "icons/atcoder.png",
  leetcode: "icons/leetcode.png",
  cses: "icons/cses.png",
};

const PLATFORM_NAMES = {
  codeforces: "Codeforces",
  atcoder: "AtCoder",
  leetcode: "LeetCode",
  cses: "CSES",
};

document.addEventListener("DOMContentLoaded", () => {
  initScraping();

  // Add Test Case Event
  document.getElementById("btn-add-case").addEventListener("click", () => {
    if (!problemData) return;

    // Add empty testcase
    problemData.tests.push({
      input: "",
      expected: "",
    });

    // Re-render and write to clipboard
    renderTestCases();
    triggerClipboardUpdate();

    // Scroll to the bottom of the test cases list
    const list = document.getElementById("testcases-list");
    if (list) {
      setTimeout(() => {
        list.scrollTop = list.scrollHeight;
      }, 50);
    }
  });
});

// Flash copy feedback indicator
function flashStatusPill() {
  const pill = document.getElementById("status-pill");
  if (!pill) return;
  pill.classList.remove("show");
  void pill.offsetWidth; // trigger reflow
  pill.classList.add("show");

  // Clear any existing timeouts on the element
  if (pill.timeoutId) {
    clearTimeout(pill.timeoutId);
  }
  pill.timeoutId = setTimeout(() => {
    pill.classList.remove("show");
  }, 1200);
}

// Copy JSON state to clipboard with debounce
function triggerClipboardUpdate() {
  if (!problemData) return;

  clearTimeout(clipboardDebounceTimer);
  clipboardDebounceTimer = setTimeout(() => {
    const jsonStr = JSON.stringify(problemData, null, 2);
    navigator.clipboard
      .writeText(jsonStr)
      .then(() => {
        flashStatusPill();
      })
      .catch((err) => {
        console.error("Clipboard write failure:", err);
      });
  }, 200); // 200ms debounce
}

// Scrape initiation pipeline
function initScraping() {
  showLoading();

  chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
    if (chrome.runtime.lastError || !tabs || tabs.length === 0) {
      showError("Could not retrieve active tab context.");
      return;
    }

    const activeTab = tabs[0];
    const url = activeTab.url || "";

    let platform = "";
    if (url.includes("codeforces.com")) {
      platform = "codeforces";
    } else if (url.includes("atcoder.jp")) {
      platform = "atcoder";
    } else if (url.includes("leetcode.com")) {
      platform = "leetcode";
    } else if (url.includes("cses.fi")) {
      platform = "cses";
    }

    if (!platform) {
      showError(
        "Please open a problem page on Codeforces, AtCoder, LeetCode, or CSES.",
      );
      return;
    }

    // Send scrape request message to content script. The very first
    // sendMessage call after the extension has been idle can fail with
    // "could not connect" purely due to a Firefox timing quirk -- the
    // internal messaging route between popup and content script isn't
    // always warmed up yet on that first call, even though the content
    // script itself is present and listening. Retrying once after a short
    // delay papers over that cold-start race instead of surfacing an
    // error the user would just "fix" by clicking again anyway.
    sendScrapeMessageWithRetry(activeTab.id, 2);
  });
}

function sendScrapeMessageWithRetry(tabId, attemptsLeft) {
  chrome.tabs.sendMessage(tabId, { action: "START_SCRAPING" }, (response) => {
    const failed = chrome.runtime.lastError || !response || !response.success;

    if (failed && attemptsLeft > 0) {
      setTimeout(() => {
        sendScrapeMessageWithRetry(tabId, attemptsLeft - 1);
      }, 250);
      return;
    }

    if (failed) {
      showError(
        "Scraper could not connect to page. Try refreshing the problem tab.",
      );
      return;
    }

    problemData = response.data;

    // Enforce default baseline contract for result collection ordering
    if (problemData.order_matters === undefined) {
      problemData.order_matters = true;
    }
    // Same for in-place detection — scraper sets these for LeetCode,
    // but guard against an older/failed scrape not setting them at all.
    if (problemData.in_place === undefined) {
      problemData.in_place = false;
    }
    if (problemData.target_param === undefined) {
      problemData.target_param = "";
    }

    // Auto-copy initial payload immediately
    triggerClipboardUpdate();

    // Display UI elements
    renderMainLayout();
  });
}

function showLoading() {
  const container = document.getElementById("app-container");
  container.innerHTML = `
    <div class="state-msg">
      <div class="spinner"></div>
      <p>Scraping problem description & cases...</p>
    </div>
  `;
  document.getElementById("actions-bar").style.display = "none";
}

function showError(message) {
  const container = document.getElementById("app-container");
  container.innerHTML = `
    <div class="state-msg">
      <div class="icon">⚠️</div>
      <p>${message}</p>
      <button class="btn-retry" id="btn-retry">Retry Scrape</button>
    </div>
  `;
  document.getElementById("actions-bar").style.display = "none";

  document.getElementById("btn-retry").addEventListener("click", () => {
    initScraping();
  });
}

// Render problem structure
function renderMainLayout() {
  const container = document.getElementById("app-container");
  if (!problemData) return;

  const platformClass = problemData.platform;
  const platformLogo = PLATFORM_ICONS[platformClass] || "";
  const platformName = PLATFORM_NAMES[platformClass] || "Platform";
  const timeLimitVal = problemData.time_limit_ms
    ? `${problemData.time_limit_ms} ms`
    : "2000 ms";

  // Determine styles inline for the sequence button state
  const buttonColor = problemData.order_matters
    ? "var(--success)"
    : "var(--danger)";
  const buttonText = problemData.order_matters
    ? "Strict Order"
    : "Any Order Allowed";

  container.innerHTML = `
    <div id="problem-card">
      <div class="meta-row">
        <span class="platform-badge ${platformClass}">
          <img src="${platformLogo}" alt="${platformName}">
          ${platformName}
        </span>
        <span class="time-limit">${timeLimitVal}</span>
      </div>
      <h2 class="problem-title">${problemData.title}</h2>

      <div class="order-toggle-row" style="display: flex; align-items: center; justify-content: space-between; margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border);">
        <span style="font-size: 11px; font-weight: 600; color: var(--subtext);">Output Sequence:</span>
        <button id="btn-order-toggle" style="background-color: var(--input-bg); color: ${buttonColor}; border: 1px solid var(--border); padding: 3px 8px; border-radius: 4px; font-size: 11px; font-weight: 700; cursor: pointer; transition: all 0.15s ease;">
          ${buttonText}
        </button>
      </div>
    </div>

    <div id="leetcode-sig-section">
      <label>Function Signature (LeetCode)</label>
      <div class="sig-code" id="sig-code-box"></div>

      <div class="inplace-row">
        <span class="inplace-row-label">
          In-Place Algorithm
        </span>
        <input type="checkbox" id="chk-inplace" class="inplace-toggle" ${problemData.in_place ? "checked" : ""}>
      </div>

      <div class="inplace-target-row ${problemData.in_place ? "show" : ""}" id="inplace-target-row">
        <label>Target Parameter</label>
        <input type="text" id="input-target-param" placeholder="e.g. nums" value="${escapeHtml(problemData.target_param || "")}">
        <span class="hint">Leave blank to auto-detect the first slice/array argument.</span>
      </div>
    </div>

    <div class="section-title">Test Cases</div>
    <div id="testcases-list"></div>
  `;

  // Bind the runtime event listener to the toggle switch button directly
  document.getElementById("btn-order-toggle").addEventListener("click", (e) => {
    problemData.order_matters = !problemData.order_matters;

    // In-place UI alterations to avoid redrawing full fields and dropping focus states
    if (problemData.order_matters) {
      e.target.innerText = "Strict Order";
      e.target.style.color = "var(--success)";
    } else {
      e.target.innerText = "Any Order Allowed";
      e.target.style.color = "var(--danger)";
    }

    // Update data object state back to system clipboard buffer
    triggerClipboardUpdate();
  });

  // Handle Leetcode function signature block representation
  const sigSection = document.getElementById("leetcode-sig-section");
  if (problemData.platform === "leetcode" && problemData.function_signature) {
    sigSection.style.display = "flex";
    document.getElementById("sig-code-box").innerText =
      problemData.function_signature;

    // In-Place toggle: flips problemData.in_place and shows/hides the
    // target-parameter field without a full re-render (keeps focus state
    // consistent with the order-toggle pattern above).
    const inplaceToggle = document.getElementById("chk-inplace");
    const targetRow = document.getElementById("inplace-target-row");
    inplaceToggle.addEventListener("change", (e) => {
      problemData.in_place = e.target.checked;
      targetRow.classList.toggle("show", e.target.checked);
      triggerClipboardUpdate();
    });

    // Target parameter field: free text, blank means "let the CLI
    // auto-detect the first slice-typed parameter".
    const targetInput = document.getElementById("input-target-param");
    targetInput.addEventListener("input", (e) => {
      problemData.target_param = e.target.value.trim();
      triggerClipboardUpdate();
    });
  } else {
    sigSection.style.display = "none";
  }

  // Display action bar (Add Test Case)
  document.getElementById("actions-bar").style.display = "flex";

  // Render sub test cases
  renderTestCases();
}

// Render individual test cases list
function renderTestCases() {
  const listContainer = document.getElementById("testcases-list");
  if (!listContainer || !problemData) return;

  if (problemData.tests.length === 0) {
    listContainer.innerHTML = `
      <div style="text-align: center; padding: 20px; font-size: 12px; color: var(--subtext); font-style: italic;">
        No test cases loaded. Click "Add Custom Case" below.
      </div>
    `;
    return;
  }

  listContainer.innerHTML = "";

  problemData.tests.forEach((test, index) => {
    const card = document.createElement("div");
    card.className = "testcase-card";
    card.innerHTML = `
      <div class="testcase-header">
        <span>Case #${index + 1}</span>
        <button class="btn-delete" data-index="${index}" title="Remove Case">&times;</button>
      </div>
      <div class="testcase-grid">
        <div class="field">
          <label>Input</label>
          <textarea class="txt-input" data-index="${index}">${escapeHtml(test.input)}</textarea>
        </div>
        <div class="field">
          <label>Expected Output</label>
          <textarea class="txt-expected" data-index="${index}">${escapeHtml(test.expected)}</textarea>
        </div>
      </div>
    `;

    listContainer.appendChild(card);
  });

  // Bind input key listeners for real-time saving and auto-copying
  const inputs = listContainer.querySelectorAll(".txt-input");
  const expecteds = listContainer.querySelectorAll(".txt-expected");

  inputs.forEach((el) => {
    el.addEventListener("input", (e) => {
      const idx = parseInt(e.target.dataset.index);
      problemData.tests[idx].input = e.target.value;
      triggerClipboardUpdate();
    });
  });

  expecteds.forEach((el) => {
    el.addEventListener("input", (e) => {
      const idx = parseInt(e.target.dataset.index);
      problemData.tests[idx].expected = e.target.value;
      triggerClipboardUpdate();
    });
  });

  // Bind Delete buttons
  const deleteBtns = listContainer.querySelectorAll(".btn-delete");
  deleteBtns.forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const idx = parseInt(e.target.dataset.index);
      problemData.tests.splice(idx, 1);
      renderTestCases();
      triggerClipboardUpdate();
    });
  });
}

function escapeHtml(text) {
  if (!text) return "";
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
