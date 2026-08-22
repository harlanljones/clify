// Winamp 2-inspired spectrum analyzer: stacked LED segments per band with
// falling peak caps. Three-tone gradient drawn over each bar (low / mid /
// high) so the column "lights up" the way the classic skin did.
//
// Driven by 10-band frames from BandStream. Colors come from the active
// Omarchy theme via NowPlaying.qml.

import QtQuick

Item {
    id: root
    property var bands: []

    // Three-tone color stack. Bottom -> top: barColor (low), accentColor
    // (mid), warnColor (top). The user passes the active theme accent,
    // yellow, and red into these.
    property color barColor:    "#a9b665"
    property color accentColor: "#d8a657"
    property color warnColor:   "#ea6962"

    // Segment geometry. `segH` and `segGap` define the LED-stack look — keep
    // segGap >= 1 so the dark line between segments stays visible.
    property int segH:   3
    property int segGap: 1
	readonly property int bandCount: Math.max(10, bands ? bands.length : 0)
	readonly property int rows: Math.max(4, Math.floor(height / (segH + segGap)))

    implicitWidth: 320
    implicitHeight: 56

    property var peaks: Array(10).fill(0)

    Timer {
        // Drives peak decay independent of band update rate. Skips the state
        // write when nothing moved so a paused player doesn't allocate and
        // emit a peaksChanged signal at 30 Hz.
		interval: 50
		running: root.visible
        repeat: true
        onTriggered: {
            const cur = root.peaks;
            const next = cur.slice();
            let dirty = false;
            for (let i = 0; i < next.length; ++i) {
                const v = root.bands[i] || 0;
                const nv = v > next[i] ? v : Math.max(0, next[i] - 0.018);
                if (nv !== cur[i]) dirty = true;
                next[i] = nv;
            }
            if (dirty) {
                root.peaks = next;
            }
        }
    }

	Row {
		anchors.fill: parent
		spacing: 2
		Repeater {
			model: root.bandCount
			Item {
				required property int index
				width: (parent.width - parent.spacing * (root.bandCount - 1)) / root.bandCount
				height: parent.height
				readonly property real value: Math.max(0, Math.min(1, root.bands[index] || 0))
				Repeater {
					model: root.rows
					Rectangle {
						required property int index
						width: parent.width
						height: root.segH
						y: parent.height - (index + 1) * (root.segH + root.segGap) + root.segGap
						visible: index < Math.round(parent.value * root.rows)
						color: index < Math.round(root.rows * 0.55) ? root.barColor
						       : index < Math.round(root.rows * 0.85) ? root.accentColor : root.warnColor
					}
				}
				Rectangle {
					width: parent.width
					height: root.segH
					visible: (root.peaks[index] || 0) > 0
					y: parent.height - Math.max(1, Math.round((root.peaks[index] || 0) * root.rows))
					   * (root.segH + root.segGap) + root.segGap
					color: root.accentColor
				}
			}
		}
    }
}
