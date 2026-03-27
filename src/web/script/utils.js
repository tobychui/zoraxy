/*
    Utils.js

    This script holds utilities function that is commonly use
    across components

*/

function abbreviateNumber(value) {
    var newValue = value;
    var suffixes = ["", "k", "m", "b", "t"];
    var suffixNum = 0;
    while (newValue >= 1000 && suffixNum < suffixes.length - 1) {
        newValue /= 1000;
        suffixNum++;
    }
    if (value > 1000){
        newValue = newValue.toFixed(2);
    }
    
    return newValue + suffixes[suffixNum];
}

Object.defineProperty(String.prototype, 'capitalize', {
    value: function() {
        return this.charAt(0).toUpperCase() + this.slice(1);
    },
    enumerable: false
});

function getCsrfToken() {
    let csrfMeta = document.querySelector('meta[name="zoraxy.csrf.Token"]');
    return csrfMeta ? csrfMeta.getAttribute("content") : "";
}

function normalizeHttpMethod(method) {
    return String(method || "GET").trim().toUpperCase();
}

function injectCsrfHeader(payload) {
    let method = normalizeHttpMethod(payload.method || payload.type);
    if (["POST", "PUT", "DELETE", "PATCH"].includes(method)) {
        let csrfToken = getCsrfToken();
        payload.headers = Object.assign({}, payload.headers || {}, {
            "X-CSRF-Token": csrfToken,
        });

        let originalBeforeSend = payload.beforeSend;
        payload.beforeSend = function(xhr) {
            if (csrfToken) {
                xhr.setRequestHeader("X-CSRF-Token", csrfToken);
            }
            if (typeof originalBeforeSend === "function") {
                return originalBeforeSend.apply(this, arguments);
            }
        };
    }
}

//Add a new function to jquery for ajax override with csrf token injected
$.cjax = function(payload){
    injectCsrfHeader(payload);
    payload.oncomplete = function(data, raws, status, xhr){
        console.log(data, raws, status, xhr)
        // if (status === 307){
        //     window.location.href = data.headers["Location"];
        // }
    }
    $.ajax(payload);
}

$.ajaxPrefilter(function(options, originalOptions, jqXHR){
    injectCsrfHeader(options);
});

let assignableNodeCache = null;
let localNodeStateCache = null;
let localNodeInfoCache = {
    nodeName: "Local",
    mode: "primary"
};

function defaultLocalNodeState() {
    return {
        name: "Local",
        management_port: "8000",
        mode: "primary",
        config_write_unlocked: false,
        config_write_allowed: true,
        config_locked: false,
        sync_interval_seconds: 60,
        acme_managed_by_primary: false,
        acme_message: ""
    };
}

function syncLocalNodeCaches(state) {
    let normalizedState = Object.assign(defaultLocalNodeState(), state || {});
    localNodeStateCache = normalizedState;
    localNodeInfoCache = {
        nodeName: normalizedState.name || "Local",
        mode: normalizedState.mode || "primary",
        hostname: normalizedState.hostname || ""
    };
    return normalizedState;
}

function isNodeModeInfo(info) {
    return !!(info && String(info.mode || "").toLowerCase() === "node");
}

function isNodeMode() {
    return isNodeModeInfo(localNodeInfoCache);
}

function getLocalNodeLabel() {
    return "Local";
}

function loadLocalNodeState(callback, forceRefresh) {
    forceRefresh = forceRefresh === true;
    if (!forceRefresh && localNodeStateCache && localNodeStateCache.name) {
        callback(localNodeStateCache);
        return;
    }

    $.get("/api/node/localstate", function(data) {
        callback(syncLocalNodeCaches(data));
    }).fail(function() {
        $.get("/api/info/x", function(data) {
            callback(syncLocalNodeCaches({
                name: (data && data.NodeName) ? data.NodeName : "Local",
                mode: (data && data.Mode) ? data.Mode : "primary",
                hostname: (data && data.Hostname) ? data.Hostname : ""
            }));
        }).fail(function() {
            callback(syncLocalNodeCaches(defaultLocalNodeState()));
        });
    });
}

function loadLocalNodeInfo(callback, forceRefresh) {
    loadLocalNodeState(function() {
        callback(localNodeInfoCache);
    }, forceRefresh);
}

function isNodeWriteLockedState(state) {
    return !!(state && String(state.mode || "").toLowerCase() === "node" && !state.config_write_unlocked);
}

function getNodeWriteLockMessage(state) {
    if (state && state.acme_message && state.acme_managed_by_primary) {
        return "This page is managed by the primary node. Open Status and enable Emergency Local Override to edit locally.";
    }
    return "This page is managed by the primary node. Open Status and enable Emergency Local Override to edit locally.";
}

function showNodeWriteLockedMessage(state, element, target) {
    let $target = target ? (target.jquery ? target : $(target)) : $(document);
    let $element = element ? (element.jquery ? element : $(element)) : $();

    if ($target.find(".nodeWriteLockNotice").length > 0) {
        return;
    }
    if ($element.length > 0 && $element.closest(".nodeWriteLockNotice").length > 0) {
        return;
    }

    msgbox(getNodeWriteLockMessage(state), false, 6000);
}

function applyNodeWriteLockState(target, options, forceRefresh) {
    let $target = target ? (target.jquery ? target : $(target)) : $(document);
    let settings = Object.assign({
        keepEnabledSelector: "[data-node-lock-exempt]"
    }, options || {});

    if (!$target.data("nodeWriteLockBound")) {
        $target.on("click", "button, .ui.button, .ui.checkbox, .ui.dropdown .item, .ui.dropdown", function(event) {
            let state = localNodeStateCache || defaultLocalNodeState();
            let $element = $(this);
            if (!isNodeWriteLockedState(state)) {
                return;
            }

            if (settings.keepEnabledSelector && ($element.is(settings.keepEnabledSelector) || $element.closest(settings.keepEnabledSelector).length > 0)) {
                return;
            }

            event.preventDefault();
            event.stopImmediatePropagation();
            showNodeWriteLockedMessage(state, $element, $target);
            return false;
        });
        $target.on("submit", "form", function(event) {
            let state = localNodeStateCache || defaultLocalNodeState();
            if (!isNodeWriteLockedState(state)) {
                return;
            }

            if (settings.keepEnabledSelector && ($(this).is(settings.keepEnabledSelector) || $(this).closest(settings.keepEnabledSelector).length > 0)) {
                return;
            }

            event.preventDefault();
            event.stopImmediatePropagation();
            showNodeWriteLockedMessage(state, $(this), $target);
            return false;
        });
        $target.data("nodeWriteLockBound", true);
    }

    loadLocalNodeState(function(state) {
        let locked = isNodeWriteLockedState(state);
        let keepEnabledSelector = settings.keepEnabledSelector || "";

        $target.find(".nodeWriteLockNotice").toggle(locked);
        if ($target.hasClass("nodeWriteLockNotice")) {
            $target.toggle(locked);
        }

        $target.find(".hideWhenNodeLocked").toggle(!locked);
        if ($target.hasClass("hideWhenNodeLocked")) {
            $target.toggle(!locked);
        }

        let $protected = $target.find(".nodeWriteProtected");
        if ($target.hasClass("nodeWriteProtected")) {
            $protected = $protected.add($target);
        }
        let $protectedScopes = $target.find(".nodeWriteProtectedScope");
        if ($target.hasClass("nodeWriteProtectedScope")) {
            $protectedScopes = $protectedScopes.add($target);
        }
        $protectedScopes.each(function() {
            $protected = $protected.add($(this).find("button, input, textarea, select, .ui.dropdown, .ui.checkbox"));
        });

        $protected.each(function() {
            let $element = $(this);
            if (keepEnabledSelector && ($element.is(keepEnabledSelector) || $element.closest(keepEnabledSelector).length > 0)) {
                return;
            }

            $element.toggleClass("disabled", locked);
            if ($element.is("button, input, textarea, select")) {
                if (locked) {
                    if ($element.data("nodeLockOriginalDisabled") === undefined) {
                        $element.data("nodeLockOriginalDisabled", $element.prop("disabled"));
                    }
                    $element.prop("disabled", true);
                } else if ($element.data("nodeLockOriginalDisabled") !== undefined) {
                    $element.prop("disabled", !!$element.data("nodeLockOriginalDisabled"));
                    $element.removeData("nodeLockOriginalDisabled");
                } else {
                    $element.prop("disabled", false);
                }
            }
        });

        if (typeof settings.onChange === "function") {
            settings.onChange(state, locked);
        }
    }, forceRefresh === true);
}

function normalizeAssignableNodes(data) {
    let nodes = [{
        id: "",
        host: "Local",
        online: true,
        status: "online"
    }];

    if (isNodeMode()) {
        return nodes;
    }

    if (Array.isArray(data)) {
        data.forEach(function(node) {
            if (!node || node.id === undefined) {
                return;
            }

            nodes.push({
                id: node.id,
                host: node.display_name || node.name || node.host || node.id,
                online: !!node.online,
                status: node.status || (node.online ? "online" : "offline")
            });
        });
    }

    return nodes;
}

function getAssignableNodeStatusIconHtml(node) {
    if (!node || node.id === "") {
        return "";
    }

    if (String(node.status || "").toLowerCase() === "online") {
        return '<i class="green circle icon"></i>';
    }

    if (String(node.status || "").toLowerCase() === "offline") {
        return '<i class="red circle icon"></i>';
    }

    return '<i class="grey circle icon"></i>';
}

function buildAssignableNodeDropdownItem(node) {
    let item = $('<div class="item"></div>');
    item.attr("data-value", node.id || "");

    if (node.id !== "") {
        item.append($(getAssignableNodeStatusIconHtml(node)));
    }

    item.append(document.createTextNode(node.host || "Local"));
    return item;
}

function loadAssignableNodes(callback, forceRefresh) {
    forceRefresh = forceRefresh === true;
    if (!forceRefresh && Array.isArray(assignableNodeCache)) {
        callback(assignableNodeCache);
        return;
    }

    loadLocalNodeInfo(function() {
        $.get("/api/nodes/list", function(data) {
            assignableNodeCache = normalizeAssignableNodes(data);
            callback(assignableNodeCache);
        }).fail(function() {
            assignableNodeCache = normalizeAssignableNodes([]);
            callback(assignableNodeCache);
        });
    });
}

function getAssignedNodeEntry(assignedNodeID) {
    assignedNodeID = (assignedNodeID || "").trim();
    if (assignedNodeID === "") {
        return {
            id: "",
            host: "Local",
            online: true,
            status: "online"
        };
    }

    if (Array.isArray(assignableNodeCache)) {
        for (let i = 0; i < assignableNodeCache.length; i++) {
            if (assignableNodeCache[i].id === assignedNodeID) {
                return assignableNodeCache[i];
            }
        }
    }

    return {
        id: assignedNodeID,
        host: assignedNodeID,
        online: false,
        status: "offline"
    };
}

function getAssignedNodeLabel(assignedNodeID, includeStatus) {
    if ((assignedNodeID || "").trim() === "") {
        return "Local";
    }
    let entry = getAssignedNodeEntry(assignedNodeID);
    let label = entry.host || assignedNodeID || getLocalNodeLabel();
    if (includeStatus) {
        return `${getAssignableNodeStatusIconHtml(entry)}${label}`;
    }
    return label;
}

function renderAssignedNodeDropdown(target, selectedNodeID, onChange, forceRefresh) {
    loadAssignableNodes(function(nodes) {
        let dropdown = $('<div class="ui fluid selection dropdown"></div>');
        let menu = $('<div class="menu"></div>');
        let isInitializing = true;

        dropdown.append('<input type="hidden" name="assignedNodeId">');
        dropdown.append('<i class="dropdown icon"></i>');
        dropdown.append('<div class="default text">Local</div>');

        nodes.forEach(function(node) {
            menu.append(buildAssignableNodeDropdownItem(node));
        });
        dropdown.append(menu);

        $(target).html(dropdown);
        dropdown.dropdown({
            onChange: function(value) {
                if (!isInitializing && typeof onChange === "function") {
                    onChange(value || "");
                }
            }
        });

        dropdown.dropdown("set selected", (selectedNodeID || "").trim());
        isInitializing = false;
    }, forceRefresh);
}

function applyNodeModeVisibility(target, forceRefresh) {
    let $target = target ? (target.jquery ? target : $(target)) : $(document);
    loadLocalNodeState(function(state) {
        let hideInNodeMode = isNodeModeInfo(state);
        $target.find(".hideInNodeMode").toggle(!hideInNodeMode);
        if ($target.hasClass("hideInNodeMode")) {
            $target.toggle(!hideInNodeMode);
        }
    }, forceRefresh === true);
}
