// Compact now-playing card for cliamp. Dense Winamp-style layout.
//
// Layout (300x72, sharp edges):
//   row 1: title (bold) ............................ time (mm:ss/mm:ss)
//   row 2: artist (dim) ............................ << play/pause >>
//   row 3: 10-band spectrum visualizer (full width)
//   row 4: thin seekable progress line
//
// Driven by an MprisPlayer for transport + position. Theme colors come from
// the active Omarchy theme at ~/.config/omarchy/current/theme/colors.toml,
// watched for changes so theme swaps update the widget live.

import Quickshell
import Quickshell.Services.Mpris
import Quickshell.Io
import QtQuick

Item {
    id: root
    property var player: null

    property color bg:     "#181616"
    property color edge:   "#0d0c0c"
    property color fg:     "#c5c9c5"
    property color dim:    "#a6a69c"
    property color accent: "#658594"
    property color green:  "#8a9a7b"
    property color yellow: "#c4b28a"
    property color red:    "#c4746e"

    FileView {
        id: themeFile
        path: (Quickshell.env("HOME") || "") + "/.config/omarchy/current/theme/colors.toml"
        watchChanges: true
        // Omarchy's theme swap does `rm -rf current/theme && mv next-theme current/theme`,
        // so there's a brief window where the file genuinely doesn't exist. Quiet the
        // log and schedule a retry instead of spamming warnings.
        printErrors: false
        onFileChanged: reload()
        onLoaded:      root._applyOmarchyTheme(text())
        onLoadFailed:  reloadTimer.restart()
    }
    Timer {
        id: reloadTimer
        interval: 400
        repeat: false
        onTriggered: themeFile.reload()
    }

    function _applyOmarchyTheme(src) {
        if (!src) return;
        const lines = String(src).split("\n");
        const re = /^\s*([A-Za-z0-9_]+)\s*=\s*"?(#?[0-9A-Fa-f]+)"?\s*$/;
        const t = {};
        for (let i = 0; i < lines.length; ++i) {
            const m = lines[i].match(re);
            if (m) t[m[1]] = m[2];
        }
        if (t.background) root.bg     = t.background;
        if (t.foreground) root.fg     = t.foreground;
        if (t.accent)     root.accent = t.accent;
        if (t.color2)     root.green  = t.color2;
        if (t.color3)     root.yellow = t.color3;
        if (t.color1)     root.red    = t.color1;
        if (t.color8)     root.dim    = t.color8;
        else              root.dim    = Qt.darker(root.fg, 1.7);
        if (t.selection_background) root.edge = t.selection_background;
        else if (t.color8)          root.edge = t.color8;
        else                        root.edge = Qt.darker(root.fg, 3.0);
    }

    readonly property bool ready:    player !== null
    readonly property bool playing:  ready && player.isPlaying
    readonly property real len:      ready && player.lengthSupported ? player.length : 0
    readonly property real progress: len > 0 ? Math.min(1, livePosition / len) : 0
    property real livePosition: 0

    Timer {
        interval: 250
        running: root.ready && root.playing
        repeat: true
        onTriggered: root.livePosition = root.player.position
    }
    Connections {
        target: root.player
        function onPlaybackStateChanged() { root.livePosition = root.player.position }
        function onTrackTitleChanged()    { root.livePosition = root.player.position }
        function onPositionChanged()      { root.livePosition = root.player.position }
    }

    function fmt(seconds) {
        if (!isFinite(seconds) || seconds < 0) return "--:--";
        const s = Math.floor(seconds);
        const m = Math.floor(s / 60);
        const r = s % 60;
        return m + ":" + (r < 10 ? "0" : "") + r;
    }

    BandStream {
        id: stream
        fps: 30
		enabled: root.visible && root.ready
    }

    Rectangle {
        anchors.fill: parent
        radius: 0
        color: Qt.rgba(root.bg.r, root.bg.g, root.bg.b, 0.94)
        border.color: root.edge
        border.width: 1
    }

    Item {
        id: inner
        anchors.fill: parent
        anchors.leftMargin: 8
        anchors.rightMargin: 8
        anchors.topMargin: 5
        anchors.bottomMargin: 5

        Text {
            id: titleT
            anchors.top: parent.top
            anchors.left: parent.left
            anchors.right: timeT.left
            anchors.rightMargin: 8
            height: 13
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
			text: root.ready ? (root.player.trackTitle || qsTr("Unknown title"))
			                 : qsTr("cliamp: not running")
            color: root.fg
            font.family: "monospace"
            font.pixelSize: 12
            font.bold: true
            textFormat: Text.PlainText
        }
        Text {
            id: timeT
            anchors.top: parent.top
            anchors.right: parent.right
            height: 13
            verticalAlignment: Text.AlignVCenter
            text: root.fmt(root.livePosition) + "/" + root.fmt(root.len)
            color: root.dim
            font.family: "monospace"
            font.pixelSize: 10
        }

        // Explicit width/height on each button overrides TransportButton's
        // implicit padding so the row stays compact. The row width tracks
        // timeT.width so the transport cluster occupies the same horizontal
        // column as the timestamp above it; spacing is computed to evenly
        // distribute the three buttons across that width.
        Row {
            id: transport
            anchors.top: titleT.bottom
            anchors.topMargin: 2
            anchors.right: parent.right
            width: timeT.width
            height: 16
            spacing: Math.max(2, (width - 52) / 2)

            TransportButton {
                width: 16; height: 16
                shape: "prev"
				accessibleName: qsTr("Previous track")
                iconSize: 10
                enabled: root.ready && root.player.canGoPrevious
                fgColor: root.dim
                hoverColor: root.yellow
                onActivated: root.player.previous()
            }
            TransportButton {
                width: 20; height: 16
                shape: root.playing ? "pause" : "play"
				accessibleName: root.playing ? qsTr("Pause") : qsTr("Play")
                iconSize: 12
                enabled: root.ready && root.player.canTogglePlaying
                fgColor: root.accent
                hoverColor: root.green
                onActivated: root.player.togglePlaying()
            }
            TransportButton {
                width: 16; height: 16
                shape: "next"
				accessibleName: qsTr("Next track")
                iconSize: 10
                enabled: root.ready && root.player.canGoNext
                fgColor: root.dim
                hoverColor: root.yellow
                onActivated: root.player.next()
            }
        }

        Text {
            id: artistT
            anchors.top: titleT.bottom
            anchors.topMargin: 2
            anchors.left: parent.left
            anchors.right: transport.left
            anchors.rightMargin: 8
            height: 16
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
            text: root.ready ? (root.player.trackArtist || "") : ""
            color: root.dim
            font.family: "monospace"
            font.pixelSize: 11
            textFormat: Text.PlainText
        }

        Visualizer {
            id: vis
            anchors.top: artistT.bottom
            anchors.topMargin: 2
            anchors.left: parent.left
            anchors.right: parent.right
            height: 22
            bands: stream.bands
            barColor:    root.green
            accentColor: root.yellow
            warnColor:   root.red
            segH: 2
            segGap: 1
        }

        Item {
            id: barWrap
            anchors.top: vis.bottom
            anchors.topMargin: 3
            anchors.left: parent.left
            anchors.right: parent.right
            height: 4

            Rectangle {
                anchors.verticalCenter: parent.verticalCenter
                width: parent.width
                height: 1
                color: root.dim
                opacity: 0.45
                radius: 0
            }
            Rectangle {
                anchors.verticalCenter: parent.verticalCenter
                height: 1
                width: parent.width * root.progress
                color: root.accent
                radius: 0
            }
            Rectangle {
                visible: root.ready && root.len > 0
                width: 4; height: 4; radius: 0
                color: root.accent
                anchors.verticalCenter: parent.verticalCenter
                x: Math.max(0, Math.min(parent.width - width,
                       parent.width * root.progress - width / 2))
            }

            MouseArea {
                anchors.fill: parent
                // Expand the hit area vertically so the 1px line is actually clickable.
                anchors.topMargin: -4
                anchors.bottomMargin: -4
                enabled: root.ready && root.player.canSeek && root.len > 0
                cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
                onClicked: (mouse) => {
                    const frac = Math.max(0, Math.min(1, mouse.x / width));
                    const target = frac * root.len;
                    root.player.position = target;
                    root.livePosition = target;
                }
            }
        }
    }
}
