# Pitch deck

- **[LLM-Reliability_Walkthrough.pptx](LLM-Reliability_Walkthrough.pptx)**: 13 slides, for the Geek Room Global AI Hackathon 2026 (Bifrost track and Tavily track). Open it in PowerPoint and start the slideshow to see the animations. A PDF export shows the slides without motion.
- **[LLM-Reliability_Walkthrough_preview.mp4](LLM-Reliability_Walkthrough_preview.mp4)**: the deck played through, exported from PowerPoint at 720p.

What the deck has:

- The cover shows the live console in light mode: a real Stale-Answer Detector report, with the Progress panel from the same run floating on top.
- After each section divider, the divider slides away diagonally up-left to show the content slide. Other slides push up like a scroll.
- On every slide, the content fades in and floats up one piece at a time, without clicks.
- The closing slide has a QR code that opens the live console.

## Rebuild it

The deck is generated from [`source/deck.json`](source/deck.json) with the [Rich627/slides-skill](https://github.com/Rich627/slides-skill) renderer. A post-processing step then enlarges the cover image, adds the QR code, and adds the transitions and animations.

```bash
cd docs/pitch/source
npm install                                     # pptxgenjs, puppeteer-core
pip install python-pptx qrcode[pil] pillow
node capture.js                                 # optional: new screenshots from the live console (uses Microsoft Edge)
python compose.py                               # optional: rebuild console-live.png from those screenshots
node <slides-skill>/scripts/build_deck.js deck.json _base.pptx
python animate.py _base.pptx ../LLM-Reliability_Walkthrough.pptx cover,cards,section,flow,chart,section,flow,quote,stats,image,table,next,closing
powershell -File render.ps1 -Pptx ../LLM-Reliability_Walkthrough.pptx -OutDir render   # PNG per slide (needs PowerPoint)
```

The last argument to `animate.py` is the slide types from `deck.json`, in order. Update it if you add or remove slides.
