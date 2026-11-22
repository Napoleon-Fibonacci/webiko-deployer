/* Webiko Deployer — vanilla client. No framework, no bundler. */
(function () {
  "use strict";

  var MAX_NAME_LENGTH = 40;
  var MIN_NAME_LENGTH = 3;
  var MAX_FILES = 100;
  var MAX_ZIP_BYTES = 10 * 1024 * 1024;
  var NAME_CHECK_DEBOUNCE_MS = 500;
  var DEPLOY_TIMEOUT_MS = 120000;
  var NAME_RULE_TEXT = "Huruf kecil, angka, tanda hubung; 3–40 karakter.";
  var NAME_TAKEN_MESSAGE = "Nama ini sudah dipakai. Coba nama lain.";
  var ZIP_ONLY_MESSAGE = "Sumber ZIP harus berformat .zip.";
  var GENERIC_MESSAGE = "Terjadi kesalahan tak terduga.";
  var DROPPED_MESSAGE = "Koneksi terputus saat deploy. Coba lagi.";
  var STAGE_INDEX = { files: 0, slot: 0, create: 1 };
  var DONE_STEP_INDEX = 2;

  var ICON = {
    check:
      '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>',
    alert:
      '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>',
    archive:
      '<svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 8V6a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v2"/><path d="M4 8h16v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z"/><path d="M10 12h4"/></svg>',
    folder:
      '<svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
    rocket:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 16V8"/><path d="m8 12 4-4 4 4"/></svg>',
    trash:
      '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16"/><path d="M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/><path d="M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13"/></svg>',
    x:
      '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 6 18 18"/><path d="M18 6 6 18"/></svg>',
  };

  function byId(id) {
    return document.getElementById(id);
  }

  function esc(value) {
    return String(value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function isValidName(value) {
    return (
      value.length >= MIN_NAME_LENGTH &&
      value.length <= MAX_NAME_LENGTH &&
      /^[a-z0-9-]+$/.test(value) &&
      value.charAt(0) !== "-" &&
      value.charAt(value.length - 1) !== "-"
    );
  }

  function isZipName(value) {
    return /\.zip$/i.test(value);
  }

  function isHtmlName(value) {
    return /\.html?$/i.test(value);
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return Math.round(bytes / 1024) + " KB";
    return (bytes / 1024 / 1024).toFixed(1) + " MB";
  }

  function pickBytes(pick) {
    if (pick.kind === "zip") return pick.file.size;
    return pick.entries.reduce(function (sum, entry) {
      return sum + entry.file.size;
    }, 0);
  }

  function pickLabel(pick) {
    if (pick.kind === "zip") return pick.file.name;
    if (pick.entries.length === 1) return pick.entries[0].path;
    return pick.entries.length + " file";
  }

  function timeoutSignal(ms) {
    if (typeof AbortSignal !== "undefined" && AbortSignal.timeout) {
      return AbortSignal.timeout(ms);
    }
    var controller = new AbortController();
    setTimeout(function () {
      controller.abort();
    }, ms);
    return controller.signal;
  }

  function DeployError(message, field) {
    this.name = "DeployError";
    this.message = message || GENERIC_MESSAGE;
    this.field = field === "name" ? "name" : "source";
  }
  DeployError.prototype = Object.create(Error.prototype);

  // ---------------------------------------------------------------------
  // Home page controller
  // ---------------------------------------------------------------------

  function initDeployer() {
    var form = byId("deploy-form");
    if (!form) return;

    var els = {
      name: byId("project-name"),
      nameStatus: byId("name-status"),
      nameError: byId("name-error"),
      sourceError: byId("source-error"),
      tabs: byId("source-tabs"),
      thumb: byId("seg-thumb"),
      tabGithub: byId("tab-github"),
      tabZip: byId("tab-zip"),
      tabPaste: byId("tab-paste"),
      panelGithub: byId("panel-github"),
      panelZip: byId("panel-zip"),
      panelPaste: byId("panel-paste"),
      githubUrl: byId("github-url"),
      code: byId("code-input"),
      dropzone: byId("dropzone"),
      pickZip: byId("pick-zip"),
      pickFolder: byId("pick-folder"),
      pickHtml: byId("pick-html"),
      inputZip: byId("input-zip"),
      inputFolder: byId("input-folder"),
      inputHtml: byId("input-html"),
      chip: byId("pick-chip"),
      chipIcon: byId("chip-icon"),
      chipName: byId("chip-name"),
      chipSize: byId("chip-size"),
      chipClear: byId("chip-clear"),
      idle: byId("action-idle"),
      idleHint: byId("idle-hint"),
      dot: byId("status-dot"),
      rail: byId("action-rail"),
      railSteps: byId("action-rail") ? byId("action-rail").querySelectorAll(".rail-step") : [],
      success: byId("action-success"),
      successLink: byId("success-link"),
      successUrl: byId("success-url"),
      successWarn: byId("success-warn"),
      successIcon: byId("success-icon"),
      deployBtn: byId("deploy-btn"),
      successActions: byId("success-actions"),
      copyBtn: byId("copy-btn"),
      copyLabel: byId("copy-label"),
      againBtn: byId("again-btn"),
      previewSlot: byId("preview-slot"),
      guideCopy: byId("guide-copy"),
      guideCopyLabel: byId("guide-copy-label"),
      guideBody: byId("guide-prompt-body"),
    };

    var tabs = ["github", "zip", "paste"];
    var state = {
      name: "",
      check: "idle",
      reason: "",
      tab: "github",
      githubUrl: "",
      code: "",
      pick: null,
      working: false,
      stageIdx: 0,
      error: null,
      result: null,
      copied: false,
      previewUrl: "",
    };
    var checkSeq = 0;
    var checkTimer = 0;
    var typeTimer = 0;

    els.inputFolder.setAttribute("webkitdirectory", "");
    els.inputFolder.setAttribute("directory", "");

    function sourceReady() {
      if (state.tab === "github") return state.githubUrl.trim().length > 0;
      if (state.tab === "paste") return state.code.trim().length > 0;
      return state.pick !== null;
    }

    function isReady() {
      return isValidName(state.name) && state.check === "ok" && sourceReady();
    }

    function hintText() {
      if (!isValidName(state.name)) return "Isi nama project";
      if (state.check === "checking") return "Memeriksa nama…";
      if (state.check !== "ok") return "Nama belum tersedia";
      if (!sourceReady()) return "Pilih sumber situs";
      return "Siap dideploy";
    }

    function markShake(el) {
      el.classList.remove("shake");
      void el.offsetWidth;
      el.classList.add("shake");
    }

    function showError(message, field) {
      state.error = { target: field === "name" ? "name" : "source", message: message };
      render();
      var el = state.error.target === "name" ? els.nameError : els.sourceError;
      markShake(el);
      if (state.error.target === "name") els.name.focus();
    }

    function renderNameStatus() {
      var html;
      if (state.check === "checking") {
        html = '<span class="text-faint check-icon">Memeriksa…</span>';
      } else if (state.check === "ok") {
        html =
          '<span class="check-icon text-ok" style="display:flex;align-items:center;gap:6px"><span class="check-path">' +
          ICON.check +
          "</span>Tersedia</span>";
      } else if (state.check === "taken") {
        html =
          '<span class="check-icon text-err" style="display:flex;align-items:center;gap:6px">' +
          ICON.alert +
          esc(state.reason || NAME_TAKEN_MESSAGE) +
          "</span>";
      } else {
        html = '<span class="text-faint">' + NAME_RULE_TEXT + "</span>";
      }
      els.nameStatus.innerHTML = html;
    }

    function renderTabs() {
      var index = tabs.indexOf(state.tab);
      els.thumb.style.transform = "translateX(" + index * 100 + "%)";
      [
        [els.tabGithub, "github"],
        [els.tabZip, "zip"],
        [els.tabPaste, "paste"],
      ].forEach(function (pair) {
        var active = pair[1] === state.tab;
        pair[0].setAttribute("aria-selected", active ? "true" : "false");
        pair[0].tabIndex = active ? 0 : -1;
      });
      els.panelGithub.hidden = state.tab !== "github";
      els.panelZip.hidden = state.tab !== "zip";
      els.panelPaste.hidden = state.tab !== "paste";
    }

    function renderChip() {
      if (state.pick) {
        els.dropzone.hidden = true;
        els.chip.hidden = false;
        els.chipIcon.innerHTML = state.pick.kind === "zip" ? ICON.archive : ICON.folder;
        els.chipName.textContent = pickLabel(state.pick);
        els.chipSize.textContent = formatBytes(pickBytes(state.pick));
      } else {
        els.dropzone.hidden = false;
        els.chip.hidden = true;
      }
    }

    function renderErrors() {
      var nameError = state.error && state.error.target === "name";
      var sourceError = state.error && state.error.target === "source";
      els.nameError.hidden = !nameError;
      els.sourceError.hidden = !sourceError;
      if (nameError) els.nameError.innerHTML = ICON.alert + "<span>" + esc(state.error.message) + "</span>";
      if (sourceError) els.sourceError.innerHTML = ICON.alert + "<span>" + esc(state.error.message) + "</span>";
    }

    function renderRail() {
      Array.prototype.forEach.call(els.railSteps, function (step, index) {
        step.setAttribute(
          "data-state",
          index < state.stageIdx ? "done" : index === state.stageIdx ? "active" : "todo"
        );
      });
    }

    function renderSuccess() {
      if (!state.result) return;
      els.successLink.href = state.result.url;
      els.successWarn.hidden = !state.result.renamed;
      els.successIcon.style.color = state.result.renamed ? "var(--warn)" : "var(--ok)";
      typeUrl(state.result.url);
    }

    function typeUrl(url) {
      clearInterval(typeTimer);
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
        els.successUrl.textContent = url;
        return;
      }
      els.successUrl.textContent = "";
      var shown = 0;
      typeTimer = setInterval(function () {
        shown += 2;
        els.successUrl.textContent = url.slice(0, shown);
        if (shown >= url.length) clearInterval(typeTimer);
      }, 24);
    }

    function renderPreview() {
      if (state.previewUrl === (state.result ? state.result.url : "")) return;
      state.previewUrl = state.result ? state.result.url : "";
      els.previewSlot.replaceChildren();
      if (!state.result) return;

      var url = state.result.url;
      var host;
      try {
        host = new URL(url).hostname;
      } catch (error) {
        host = url;
      }
      var sources = [
        "https://api.microlink.io/?url=" +
          encodeURIComponent(url) +
          "&screenshot=true&meta=false&embed=screenshot.url",
        "https://image.thum.io/get/width/1200/crop/750/noanimate/" + encodeURIComponent(url),
      ];

      var link = document.createElement("a");
      link.className = "preview row-in";
      link.href = url;
      link.target = "_blank";
      link.rel = "noreferrer";
      link.referrerPolicy = "no-referrer";
      link.setAttribute("aria-label", "Buka " + host);

      var note = document.createElement("span");
      note.className = "preview-note";
      note.textContent = "Menyiapkan pratinjau…";

      var img = document.createElement("img");
      img.className = "preview-img";
      img.alt = "Pratinjau " + host;
      img.decoding = "async";
      img.src = sources[0];

      var index = 0;
      img.addEventListener("load", function () {
        img.classList.add("is-loaded");
        note.hidden = true;
      });
      img.addEventListener("error", function () {
        if (index + 1 < sources.length) {
          index += 1;
          img.src = sources[index];
          return;
        }
        img.remove();
        note.textContent = "Pratinjau tidak tersedia — buka " + host;
      });

      link.appendChild(note);
      link.appendChild(img);
      els.previewSlot.appendChild(link);
    }

    function render() {
      if (els.name.value !== state.name) els.name.value = state.name;
      if (els.githubUrl.value !== state.githubUrl) els.githubUrl.value = state.githubUrl;
      if (els.code.value !== state.code) els.code.value = state.code;

      renderNameStatus();
      renderTabs();
      renderChip();
      renderErrors();
      renderRail();

      var ready = isReady();
      var success = !!state.result && !state.working;

      els.dot.classList.toggle("is-ready", ready && !state.working && !success);
      els.idleHint.textContent = hintText();

      els.idle.hidden = state.working || success;
      els.rail.hidden = !state.working;
      els.success.hidden = !success;
      els.deployBtn.hidden = success;
      els.successActions.hidden = !success;
      els.deployBtn.disabled = state.working || !ready;
      els.deployBtn.innerHTML = state.working
        ? '<span class="spinner" aria-hidden="true"></span>Mendeploy…'
        : ICON.rocket + "Deploy";

      if (success) renderSuccess();
      renderPreview();
    }

    function scheduleCheck() {
      clearTimeout(checkTimer);
      checkSeq += 1;
      var seq = checkSeq;

      if (!isValidName(state.name)) {
        state.check = "idle";
        state.reason = "";
        render();
        return;
      }
      state.check = "checking";
      state.reason = "";
      render();

      checkTimer = setTimeout(function () {
        fetch("/api/check-name?name=" + encodeURIComponent(state.name), {
          signal: timeoutSignal(10000),
        })
          .then(function (response) {
            return response.json();
          })
          .then(function (data) {
            if (seq !== checkSeq) return;
            if (data.available) {
              state.check = "ok";
              state.reason = "";
            } else {
              state.check = "taken";
              state.reason = data.reason || NAME_TAKEN_MESSAGE;
            }
            render();
          })
          .catch(function () {
            if (seq !== checkSeq) return;
            state.check = "idle";
            state.reason = "";
            render();
          });
      }, NAME_CHECK_DEBOUNCE_MS);
    }

    function setTab(tab) {
      state.tab = tab;
      render();
    }

    function setPick(pick) {
      state.pick = pick;
      render();
    }

    function clearErrors() {
      if (!state.error) return;
      state.error = null;
      render();
    }

    els.name.addEventListener("input", function () {
      state.name = els.name.value
        .toLowerCase()
        .replace(/[^a-z0-9-]/g, "")
        .slice(0, MAX_NAME_LENGTH);
      els.name.value = state.name;
      clearErrors();
      scheduleCheck();
    });

    els.tabs.addEventListener("keydown", function (event) {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      event.preventDefault();
      var current = tabs.indexOf(state.tab);
      var next =
        event.key === "ArrowRight"
          ? tabs[(current + 1) % tabs.length]
          : tabs[(current + tabs.length - 1) % tabs.length];
      setTab(next);
      byId("tab-" + next).focus();
    });

    els.tabGithub.addEventListener("click", function () {
      setTab("github");
    });
    els.tabZip.addEventListener("click", function () {
      setTab("zip");
    });
    els.tabPaste.addEventListener("click", function () {
      setTab("paste");
    });

    els.githubUrl.addEventListener("input", function () {
      state.githubUrl = els.githubUrl.value;
      clearErrors();
      render();
    });

    els.code.addEventListener("input", function () {
      state.code = els.code.value;
      clearErrors();
      render();
    });

    els.pickZip.addEventListener("click", function (event) {
      event.stopPropagation();
      els.inputZip.click();
    });
    els.pickFolder.addEventListener("click", function (event) {
      event.stopPropagation();
      els.inputFolder.click();
    });
    els.pickHtml.addEventListener("click", function (event) {
      event.stopPropagation();
      els.inputHtml.click();
    });

    els.dropzone.addEventListener("click", function () {
      els.inputZip.click();
    });
    els.dropzone.addEventListener("keydown", function (event) {
      if (event.target !== els.dropzone) return;
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        els.inputZip.click();
      }
    });
    els.dropzone.addEventListener("dragover", function (event) {
      event.preventDefault();
      els.dropzone.classList.add("is-over");
    });
    els.dropzone.addEventListener("dragleave", function (event) {
      if (!els.dropzone.contains(event.relatedTarget)) {
        els.dropzone.classList.remove("is-over");
      }
    });
    els.dropzone.addEventListener("drop", function (event) {
      event.preventDefault();
      els.dropzone.classList.remove("is-over");
      void handleDrop(event.dataTransfer);
    });

    els.inputZip.addEventListener("change", function () {
      var file = els.inputZip.files && els.inputZip.files[0];
      if (file && !isZipName(file.name)) {
        showError(ZIP_ONLY_MESSAGE);
        els.inputZip.value = "";
        return;
      }
      setPick(file ? { kind: "zip", file: file } : null);
    });

    els.inputFolder.addEventListener("change", function () {
      var list = Array.prototype.slice.call(els.inputFolder.files || []);
      if (list.length === 0) {
        setPick(null);
        return;
      }
      setPick({
        kind: "folder",
        entries: list.map(function (file) {
          return { path: file.webkitRelativePath || file.name, file: file };
        }),
      });
    });

    els.inputHtml.addEventListener("change", function () {
      var file = els.inputHtml.files && els.inputHtml.files[0];
      if (!file) return;
      if (!isHtmlName(file.name)) {
        showError("Sumber HTML harus berformat .html atau .htm.");
        els.inputHtml.value = "";
        return;
      }
      setPick({ kind: "folder", entries: [{ path: file.name, file: file }] });
    });

    els.chipClear.addEventListener("click", function () {
      els.inputZip.value = "";
      els.inputFolder.value = "";
      setPick(null);
    });

    async function walkEntry(entry, prefix, out) {
      if (entry.isFile) {
        await new Promise(function (resolve) {
          entry.file(
            function (file) {
              out.push({ path: prefix + file.name, file: file });
              resolve();
            },
            function () {
              resolve();
            }
          );
        });
        return;
      }
      var reader = entry.createReader();
      await new Promise(function (resolve) {
        var readBatch = function () {
          reader.readEntries(
            function (batch) {
              if (batch.length === 0) {
                resolve();
                return;
              }
              Promise.all(
                batch.map(function (child) {
                  return walkEntry(child, prefix + entry.name + "/", out);
                })
              ).then(readBatch, resolve);
            },
            function () {
              resolve();
            }
          );
        };
        readBatch();
      });
    }

    async function handleDrop(dataTransfer) {
      var entries = [];
      var walkers = [];
      Array.prototype.forEach.call(dataTransfer.items || [], function (item) {
        if (item.kind !== "file") return;
        var entry = item.webkitGetAsEntry && item.webkitGetAsEntry();
        if (entry) walkers.push(walkEntry(entry, "", entries));
      });
      await Promise.all(walkers);

      var fallback = Array.prototype.map.call(dataTransfer.files || [], function (file) {
        return { path: file.name, file: file };
      });
      var all = entries.length > 0 ? entries : fallback;
      if (all.length === 0) return;

      setTab("zip");
      if (all.length === 1 && isZipName(all[0].path)) {
        setPick({ kind: "zip", file: all[0].file });
      } else {
        setPick({ kind: "folder", entries: all });
      }
    }

    function pastedPick(code) {
      return {
        kind: "folder",
        entries: [
          { path: "index.html", file: new File([code], "index.html", { type: "text/html" }) },
        ],
      };
    }

    async function toZipFile(pick) {
      if (!pick) throw new DeployError("Pilih file ZIP atau folder dulu.", "source");
      if (pick.kind === "zip") {
        if (!isZipName(pick.file.name)) throw new DeployError(ZIP_ONLY_MESSAGE, "source");
        if (pick.file.size > MAX_ZIP_BYTES) {
          throw new DeployError("ZIP lebih dari " + MAX_ZIP_BYTES / 1024 / 1024 + " MB.", "source");
        }
        return pick.file;
      }
      if (pick.entries.length > MAX_FILES) {
        throw new DeployError("Folder berisi lebih dari " + MAX_FILES + " file.", "source");
      }
      var entries = {};
      var total = 0;
      for (var i = 0; i < pick.entries.length; i += 1) {
        var entry = pick.entries[i];
        total += entry.file.size;
        if (total > MAX_ZIP_BYTES) {
          throw new DeployError(
            "Isi folder melebihi " + MAX_ZIP_BYTES / 1024 / 1024 + " MB.",
            "source"
          );
        }
        entries[entry.path] = new Uint8Array(await entry.file.arrayBuffer());
      }
      var zipped = fflate.zipSync(entries);
      return new File([zipped], "folder.zip", { type: "application/zip" });
    }

    function safePathname(name) {
      return (
        "uploads/" +
        Date.now() +
        "-" +
        String(name)
          .replace(/[^A-Za-z0-9._-]/g, "-")
          .slice(-80)
      );
    }

    async function uploadZip(file) {
      var pathname = safePathname(file.name);
      var tokenResponse = await fetch("/api/upload", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          type: "blob.generate-client-token",
          payload: {
            pathname: pathname,
            callbackUrl: window.location.origin + "/api/upload",
            clientPayload: null,
            multipart: false,
          },
        }),
      });
      if (!tokenResponse.ok) {
        var tokenError = await tokenResponse.json().catch(function () {
          return null;
        });
        throw new DeployError((tokenError && tokenError.error) || "Gagal menyiapkan unggahan.", "source");
      }
      var tokenData = await tokenResponse.json();
      if (!tokenData.clientToken) {
        throw new DeployError("Gagal menyiapkan unggahan.", "source");
      }

      var putResponse = await fetch(
        "https://vercel.com/api/blob/?" + new URLSearchParams({ pathname: pathname }).toString(),
        {
          method: "PUT",
          headers: {
            authorization: "Bearer " + tokenData.clientToken,
            "x-api-version": "11",
            "x-content-type": "application/zip",
          },
          body: file,
        }
      );
      if (!putResponse.ok) {
        throw new DeployError("Gagal mengunggah ZIP. Coba lagi.", "source");
      }
      var uploaded = await putResponse.json();
      if (!uploaded.url) {
        throw new DeployError("Gagal mengunggah ZIP. Coba lagi.", "source");
      }
      return uploaded.url;
    }

    async function prepareSource() {
      if (state.tab === "github") {
        return { type: "github", url: state.githubUrl.trim() };
      }
      var pick = state.tab === "paste" ? pastedPick(state.code) : state.pick;
      var file = await toZipFile(pick);
      var blobUrl = await uploadZip(file);
      return { type: "zip", blobUrl: blobUrl };
    }

    async function streamEvents(body, handlers) {
      var reader = body.getReader();
      var decoder = new TextDecoder();
      var buffer = "";
      var finished = false;

      while (!finished) {
        var chunk = await reader.read();
        if (chunk.done) break;
        buffer += decoder.decode(chunk.value, { stream: true });
        var newline = buffer.indexOf("\n");
        while (newline >= 0) {
          var line = buffer.slice(0, newline).trim();
          buffer = buffer.slice(newline + 1);
          newline = buffer.indexOf("\n");
          if (!line) continue;
          var event;
          try {
            event = JSON.parse(line);
          } catch (error) {
            continue;
          }
          if (event.type === "stage" && handlers.onStage) {
            handlers.onStage(event.stage);
          } else if (event.type === "done" && handlers.onDone) {
            handlers.onDone(event);
            finished = true;
            break;
          } else if (event.type === "error" && handlers.onError) {
            handlers.onError(event);
            finished = true;
            break;
          }
        }
      }
      return finished;
    }

    async function runDeploy() {
      if (state.working || !isReady()) return;
      state.working = true;
      state.error = null;
      state.result = null;
      state.stageIdx = 0;
      clearInterval(typeTimer);
      render();

      var controller = new AbortController();
      var timer = 0;

      try {
        var source = await prepareSource();
        timer = setTimeout(function () {
          controller.abort();
        }, DEPLOY_TIMEOUT_MS);
        var response = await fetch("/api/deploy", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ name: state.name, source: source }),
          signal: controller.signal,
        });

        if (!response.ok || !response.body) {
          var payload = await response.json().catch(function () {
            return null;
          });
          throw new DeployError((payload && payload.error) || GENERIC_MESSAGE, "source");
        }

        var finished = await streamEvents(response.body, {
          onStage: function (stage) {
            state.stageIdx = STAGE_INDEX[stage] !== undefined ? STAGE_INDEX[stage] : 0;
            render();
          },
          onDone: function (event) {
            state.result = { url: event.url || "", renamed: event.renamed === true };
            state.stageIdx = DONE_STEP_INDEX;
            state.working = false;
            render();
          },
          onError: function (event) {
            throw new DeployError(event.message, event.field);
          },
        });

        if (!finished) throw new DeployError(DROPPED_MESSAGE, "source");
      } catch (error) {
        if (error && error.name === "AbortError") {
          showError("Deploy melewati batas waktu. Coba lagi.", "source");
        } else if (error instanceof DeployError) {
          showError(error.message, error.field);
        } else {
          showError(GENERIC_MESSAGE, "source");
        }
      } finally {
        clearTimeout(timer);
        if (state.working) {
          state.working = false;
          render();
        }
      }
    }

    form.addEventListener("submit", function (event) {
      event.preventDefault();
      void runDeploy();
    });

    els.copyBtn.addEventListener("click", async function () {
      if (!state.result) return;
      try {
        await navigator.clipboard.writeText(state.result.url);
        state.copied = true;
        els.copyLabel.textContent = "Tersalin";
        els.copyBtn.innerHTML = ICON.check + '<span id="copy-label">Tersalin</span>';
        els.copyLabel = byId("copy-label");
        setTimeout(function () {
          state.copied = false;
          els.copyBtn.innerHTML = '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg><span id="copy-label">Salin link</span>';
          els.copyLabel = byId("copy-label");
        }, 1500);
      } catch (error) {
        // Clipboard ditolak browser: biarkan.
      }
    });

    els.againBtn.addEventListener("click", function () {
      clearInterval(typeTimer);
      state.result = null;
      state.error = null;
      state.working = false;
      state.stageIdx = 0;
      state.name = "";
      state.check = "idle";
      state.reason = "";
      render();
      els.name.focus();
    });

    if (els.guideCopy && els.guideBody) {
      els.guideCopy.addEventListener("click", async function () {
        try {
          await navigator.clipboard.writeText(els.guideBody.textContent);
          var original = els.guideCopyLabel.textContent;
          els.guideCopyLabel.textContent = "Tersalin";
          setTimeout(function () {
            els.guideCopyLabel.textContent = original;
          }, 1800);
        } catch (error) {
          // Clipboard ditolak: teks tetap bisa disalin manual.
        }
      });
    }

    render();
  }

  // ---------------------------------------------------------------------
  // Admin two-step delete
  // ---------------------------------------------------------------------

  function initAdminDelete() {
    Array.prototype.forEach.call(document.querySelectorAll(".delete-trigger"), function (trigger) {
      trigger.addEventListener("click", function () {
        var cell = trigger.parentElement;
        var row = {
          id: trigger.getAttribute("data-id"),
          name: trigger.getAttribute("data-name") || "",
        };

        var form = document.createElement("form");
        form.action = "/api/admin/delete";
        form.method = "post";
        form.className = "del-confirm pop";

        var hidden = document.createElement("input");
        hidden.type = "hidden";
        hidden.name = "id";
        hidden.value = row.id;

        var ask = document.createElement("span");
        ask.className = "del-ask";
        ask.textContent = "Hapus?";

        var confirm = document.createElement("button");
        confirm.type = "submit";
        confirm.className = "icon-btn is-danger";
        confirm.title = "Ya, hapus";
        confirm.setAttribute("aria-label", "Ya, hapus " + row.name);
        confirm.innerHTML = ICON.check;

        var cancel = document.createElement("button");
        cancel.type = "button";
        cancel.className = "icon-btn";
        cancel.title = "Batal";
        cancel.setAttribute("aria-label", "Batal menghapus");
        cancel.innerHTML = ICON.x;

        form.appendChild(hidden);
        form.appendChild(ask);
        form.appendChild(confirm);
        form.appendChild(cancel);

        trigger.hidden = true;
        cell.appendChild(form);
        confirm.focus();

        cancel.addEventListener("click", function () {
          form.remove();
          trigger.hidden = false;
          trigger.focus();
        });
      });
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () {
      initDeployer();
      initAdminDelete();
    });
  } else {
    initDeployer();
    initAdminDelete();
  }
})();
