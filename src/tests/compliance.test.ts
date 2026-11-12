import { describe, it, expect } from 'vitest';
import fs from 'fs';
import path from 'path';

function getAllSourceFiles(dir: string, fileList: string[] = []): string[] {
  const files = fs.readdirSync(dir);
  for (const file of files) {
    const filePath = path.join(dir, file);
    if (fs.statSync(filePath).isDirectory()) {
      if (file !== 'tests' && file !== 'node_modules' && file !== 'dist') {
        getAllSourceFiles(filePath, fileList);
      }
    } else if (file.endsWith('.ts') || file.endsWith('.tsx') || file.endsWith('.css')) {
      fileList.push(filePath);
    }
  }
  return fileList;
}

describe('Design system compliance (§2 and §17)', () => {
  const srcDir = path.resolve(__dirname, '..');
  const sourceFiles = getAllSourceFiles(srcDir);

  it('contains no banned CSS keywords in source files', () => {
    const bannedPatterns = [
      /\bgradient\b/i,
      /\bbackdrop-blur\b/i,
      /\bshadow-lg\b/i,
      /\brounded-xl\b/i,
      /\brounded-2xl\b/i,
      /\buppercase\b/i,
      /\btracking-wide\b/i,
      /\bpurple\b/i,
      /\bviolet\b/i,
      /\bindigo\b/i,
    ];

    for (const file of sourceFiles) {
      if (file.endsWith('.test.ts')) continue;
      const content = fs.readFileSync(file, 'utf-8');
      for (const pattern of bannedPatterns) {
        expect(
          pattern.test(content),
          `Found banned keyword ${pattern} in ${path.relative(srcDir, file)}`
        ).toBe(false);
      }
    }
  });

  it('contains no banned icons or icon libraries', () => {
    const bannedLibs = ['lucide', 'heroicons', 'feather-icons', '@fortawesome'];
    for (const file of sourceFiles) {
      if (file.endsWith('.test.ts')) continue;
      const content = fs.readFileSync(file, 'utf-8');
      for (const lib of bannedLibs) {
        expect(
          content.includes(lib),
          `Found banned icon library reference "${lib}" in ${path.relative(srcDir, file)}`
        ).toBe(false);
      }
    }
  });

  it('contains no hex colors outside tokens.css', () => {
    const hexPattern = /#[0-9a-fA-F]{3,8}\b/;
    for (const file of sourceFiles) {
      if (file.endsWith('.test.ts') || file.endsWith('tokens.css')) continue;
      const content = fs.readFileSync(file, 'utf-8');
      const match = content.match(hexPattern);
      expect(
        match,
        `Found hex color "${match?.[0]}" in ${path.relative(srcDir, file)} (all colors must come from tokens.css)`
      ).toBeNull();
    }
  });

  it('contains no banned marketing words', () => {
    const bannedWords = [
      /\bUnlock\b/i,
      /\bSupercharge\b/i,
      /\bSeamless\b/i,
      /\bPowerful\b/i,
      /\bAI-powered\b/i,
      /\bMagic\b/i,
      /\bEffortless\b/i,
      /\bRevolutionize\b/i,
      /\bElevate\b/i,
    ];

    for (const file of sourceFiles) {
      if (file.endsWith('.test.ts')) continue;
      const content = fs.readFileSync(file, 'utf-8');
      for (const word of bannedWords) {
        expect(
          word.test(content),
          `Found banned marketing word ${word} in ${path.relative(srcDir, file)}`
        ).toBe(false);
      }
    }
  });

  it('contains no emoji characters in code or copy', () => {
    const emojiRegex = /[\u{1F300}-\u{1F9FF}\u{2600}-\u{26FF}]/u;
    for (const file of sourceFiles) {
      if (file.endsWith('.test.ts')) continue;
      const content = fs.readFileSync(file, 'utf-8');
      expect(
        emojiRegex.test(content),
        `Found emoji in ${path.relative(srcDir, file)}`
      ).toBe(false);
    }
  });
});

