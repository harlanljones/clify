// Long-lived `cliamp visstream` Process that parses one NDJSON frame per
// line and exposes the latest bands + visualizer mode as reactive properties.
//
// Uses imperative running control (not binding) so the respawn timer can
// flip the process back on after cliamp restarts.

import Quickshell.Io
import QtQuick

Item {
    id: root
    property int fps: 30
    property bool enabled: true

    property var bands: []
    property string mode: ""

    function _parseLine(line) {
        if (!line) return;
        try {
            const resp = JSON.parse(line);
            if (!resp || !resp.ok) return;
            if (resp.bands) root.bands = resp.bands;
            if (resp.visualizer) root.mode = resp.visualizer;
        } catch (e) { /* ignore parse errors */ }
    }

    Process {
        id: proc
        command: ["cliamp", "visstream", "--fps", String(root.fps)]
        running: false
        stdout: SplitParser {
            splitMarker: "\n"
            onRead: (line) => root._parseLine(line)
        }
    }

    Component.onCompleted: if (root.enabled) proc.running = true
    onEnabledChanged: proc.running = root.enabled

    // Respawn loop: if cliamp wasn't up yet (or restarted), keep retrying.
    Timer {
        interval: 2000
        running: root.enabled && !proc.running
        repeat: true
        onTriggered: proc.running = true
    }
}
