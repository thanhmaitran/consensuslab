#!/usr/bin/env python3
"""Draw docs/images/architecture.png from the package layout of this repository.

The boxes correspond to real directories (cmd/, internal/*, web/). Requires matplotlib.
Usage: python scripts/make_architecture_diagram.py
"""
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.patches import FancyBboxPatch

OUT = Path(__file__).resolve().parent.parent / "docs" / "images" / "architecture.png"
fig, ax = plt.subplots(figsize=(11, 6.6))
ax.set_xlim(0, 110)
ax.set_ylim(0, 66)
ax.axis("off")


def box(x, y, w, h, title, sub="", fc="#EEF3FA", ec="#4C78A8", bold=True, fs=9.5):
    ax.add_patch(FancyBboxPatch((x, y), w, h, boxstyle="round,pad=0.3,rounding_size=1.2", fc=fc, ec=ec, lw=1.4))
    if sub:
        ax.text(x + w / 2, y + h * 0.64, title, ha="center", va="center", fontsize=fs, weight="bold" if bold else "normal")
        ax.text(x + w / 2, y + h * 0.28, sub, ha="center", va="center", fontsize=7.4, color="#444")
    else:
        ax.text(x + w / 2, y + h / 2, title, ha="center", va="center", fontsize=fs, weight="bold" if bold else "normal")


def arrow(x1, y1, x2, y2, label="", both=False, dy=1.2, ha="center"):
    style = "<->" if both else "->"
    ax.annotate("", xy=(x2, y2), xytext=(x1, y1), arrowprops=dict(arrowstyle=style, color="#333", lw=1.3))
    if label:
        if x1 == x2:  # vertical arrow: put the label to the right of it
            ax.text(x1 + 1.5, (y1 + y2) / 2, label, ha="left", va="center", fontsize=7.6, color="#222")
        else:
            ax.text((x1 + x2) / 2, (y1 + y2) / 2 + dy, label, ha=ha, va="bottom", fontsize=7.6, color="#222")


# Top row: clients
box(4, 52, 34, 11, "React + TypeScript dashboard", "web/  -  Overview, Timeline, Experiments, Safety", fc="#FFF4E0", ec="#F58518")
box(44, 52, 28, 11, "CLI", "cmd/consensuslab\nsimulate | replay | experiment | serve", fc="#FFF4E0", ec="#F58518")

# API server
box(4, 37, 34, 10, "API server", "internal/api  -  REST + Server-Sent Events\nper-cluster lock, request limits", fc="#EAF6EA", ec="#54A24B")
arrow(21, 52, 21, 47.4, "HTTP  /api/v1", both=True, dy=0.2, ha="left")
arrow(58, 52, 40, 45, "serve", dy=0.2)

# Experiment runner
box(78, 37, 28, 10, "Experiment runner", "internal/experiment\nJSON scenarios, seeded runs", fc="#EAF6EA", ec="#54A24B")
arrow(66, 52, 85, 47.4, "experiment / simulate", dy=0.4, ha="left")
box(78, 22, 28, 9, "Results", "runs.jsonl | summary.csv | report.md", fc="#F5F5F5", ec="#888")
arrow(92, 37, 92, 31.4, "write")

# Simulation engine
ax.add_patch(FancyBboxPatch((4, 4), 70, 28, boxstyle="round,pad=0.3,rounding_size=1.5", fc="#F7F9FC", ec="#4C78A8", lw=1.8))
ax.text(7, 29.2, "Simulation engine  -  internal/sim", fontsize=10, weight="bold", color="#2d4a6e")
box(7, 17.5, 20, 8.5, "Clock + event queue", "virtual time, ordered by\n(time, sequence)", fc="#FFFFFF")
box(30, 17.5, 20, 8.5, "Network model", "internal/network\ndelay, jitter, loss, partition", fc="#FFFFFF")
box(53, 17.5, 18.5, 8.5, "Raft nodes", "internal/raft\nelection, replication, commit", fc="#FFFFFF")
box(7, 6.5, 20, 8.5, "Safety verifier", "internal/verify\n5 invariants", fc="#FFFFFF")
box(30, 6.5, 41.5, 8.5, "Observability", "internal/observability  -  structured events, trace hash, statistics", fc="#FFFFFF")

arrow(21, 37, 21, 32.4, "commands, faults, snapshots", both=True)
arrow(80, 37, 76, 28, "", dy=0)
ax.text(77.5, 33.2, "run engine", fontsize=7.6, ha="right")
arrow(27.4, 21.7, 29.6, 21.7, both=True, dy=0)
arrow(50.4, 21.7, 53, 21.7, both=True, dy=0)

fig.suptitle("ConsensusLab - system architecture (derived from the repository layout)", fontsize=11.5, weight="bold", y=0.985)
ax.text(55, 1.0, "Dashboard and CLI never write Raft state: they send commands and faults to the engine, which owns time, the queue, the network and the nodes.",
        ha="center", fontsize=7.6, color="#444")
OUT.parent.mkdir(parents=True, exist_ok=True)
fig.savefig(OUT, dpi=160, bbox_inches="tight")
print("wrote", OUT)
