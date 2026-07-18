// content/leetcode.js
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.action === "START_SCRAPING") {
    scrapeLeetCode().then((payload) => {
      if (payload) {
        sendResponse({ success: true, data: payload });
      } else {
        sendResponse({
          success: false,
          error: "Failed to scrape LeetCode problem page.",
        });
      }
    });
  }
  return true; // Keep channel open
});

async function scrapeLeetCode() {
  try {
    const title =
      document.querySelector("span.text-title-large")?.innerText ||
      "LeetCode Problem";

    // Extract test cases from text sample code boxes
    const preBlocks = document.querySelectorAll(
      'div[data-track-load="description_content"] pre',
    );
    let tests = [];

    preBlocks.forEach((block) => {
      const text = block.innerText;
      if (text.includes("Input:") && text.includes("Output:")) {
        const inputMatch = text.match(/Input:\s*([\s\S]*?)(?=Output:)/);
        const outputMatch = text.match(
          /Output:\s*([\s\S]*?)(?=Explanation:|Example|\n\n|$)/,
        );

        if (inputMatch && outputMatch) {
          tests.push({
            input: inputMatch[1].trim() + "\n",
            expected: outputMatch[1].trim() + "\n",
          });
        }
      }
    });

    if (tests.length === 0) {
      return null;
    }

    // Smart Data Structure verification via Topic Pill tags
    let dataStructure = "standard";
    const topicLinks = document.querySelectorAll("a");

    for (let link of topicLinks) {
      const href = link.href.toLowerCase();
      const tagText = link.innerText.toLowerCase();

      if (href.includes("/tag/binary-tree") || tagText === "binary tree") {
        dataStructure = "binary_tree";
        break;
      } else if (
        href.includes("/tag/linked-list") ||
        tagText === "linked list"
      ) {
        dataStructure = "linked_list";
        break;
      }
    }

    // Get snippets from GraphQL API using URL titleSlug
    // URL pattern is https://leetcode.com/problems/two-sum/
    const slug = window.location.pathname.split("/")[2];
    let signature = "";
    if (slug) {
      const snippets = await getSnippets(slug);
      // Try to find Go/Golang snippet first
      const goSnippet = snippets.find((s) => s.langSlug === "golang");
      if (goSnippet && goSnippet.code) {
        signature = extractSignature(goSnippet.code);
      } else {
        // Fallback to any snippet if Go is not available
        const firstSnippet = snippets[0];
        if (firstSnippet && firstSnippet.code) {
          signature = extractSignature(firstSnippet.code);
        }
      }
    }

    // If GraphQL failed or returned empty signature, fall back to editor scraping
    if (!signature) {
      const editorCode = getLeetCodeCode();
      signature = extractSignature(editorCode);
    }

    return {
      platform: "leetcode",
      title: title,
      type: "function",
      data_structure: dataStructure,
      time_limit_ms: 2000,
      memory_limit_mb: 256,
      tests: tests,
      function_signature: signature,
    };
  } catch (err) {
    console.error("LeetCode scraping failed:", err);
    return null;
  }
}

async function getSnippets(titleSlug) {
  try {
    const res = await fetch("https://leetcode.com/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({
        query: `
          query questionEditorData($titleSlug: String!) {
            question(titleSlug: $titleSlug) {
              codeSnippets {
                lang
                langSlug
                code
              }
            }
          }`,
        variables: { titleSlug },
      }),
    });
    const data = await res.json();
    return data?.data?.question?.codeSnippets || [];
  } catch (err) {
    console.error("Failed to fetch code snippets from GraphQL:", err);
    return [];
  }
}

function getLeetCodeCode() {
  // Try CodeMirror 6 (new LeetCode UI)
  const cmContent = document.querySelector(".cm-content");
  if (cmContent) {
    return cmContent.innerText;
  }
  // Try Monaco Editor
  const monacoLines = document.querySelectorAll(".view-line");
  if (monacoLines && monacoLines.length > 0) {
    return Array.from(monacoLines)
      .map((l) => l.innerText)
      .join("\n");
  }
  // Alternate selectors
  const codeMirrorLines = document.querySelectorAll(".CodeMirror-line");
  if (codeMirrorLines && codeMirrorLines.length > 0) {
    return Array.from(codeMirrorLines)
      .map((l) => l.innerText)
      .join("\n");
  }
  return "";
}

function extractSignature(code) {
  if (!code) return "";
  const lines = code.split("\n");
  for (let line of lines) {
    const trimmed = line.trim();
    // Python
    if (trimmed.startsWith("def ")) {
      return trimmed.replace(/:$/, "");
    }
    // C++ / Java / C# / JS / TS / Go / Rust
    if (
      trimmed.includes("(") &&
      !trimmed.startsWith("class ") &&
      !trimmed.startsWith("public class ") &&
      !trimmed.includes("Solution") &&
      !trimmed.startsWith("using ") &&
      !trimmed.startsWith("#") &&
      !trimmed.startsWith("import ") &&
      !trimmed.startsWith("package ")
    ) {
      return trimmed.replace(/\s*\{$/, "");
    }
  }
  // Fallback
  for (let line of lines) {
    const trimmed = line.trim();
    if (trimmed.includes("(")) {
      return trimmed.replace(/\s*\{$/, "");
    }
  }
  return "";
}
