# Fabric Mod Bisect Tool - UI Design Document

## Executive Summary

This document outlines a comprehensive redesign of the Fabric Mod Bisect Tool's GUI using only Fyne's standard layouts (HBox, VBox, Border, Center, Grid, GridWrap, Padded, Form, Scroll, Stack) to create a more intuitive, professional, and efficient user experience.

---

## Global Design Specifications

### Window Configuration

**Window Title:** "Mod Bisect Tool"
**Default Window Size:** 800 pixels width × 600 pixels height
**Minimum Window Size:** 600 pixels width × 400 pixels height

### Typography

**Font Family:** System font (Inter on Windows, San Francisco on macOS, Segoe UI on Windows)
**Monospace Font:** JetBrains Mono or Consolas for mod names

**Font Sizes:**
- Screen Title: 24pt, Bold
- Section Header: 18pt, Semi-Bold
- Primary Text: 14pt, Regular
- Secondary Text: 12pt, Regular
- Small Text: 11pt, Regular
- Button Text: 14pt, Semi-Bold
- Progress Percentage: 12pt, Regular

### Color Palette

**Background Colors:**
- Primary Background: #1e1e1e (Dark Gray)
- Surface/Card Background: #2d2d2d (Lighter Gray)
- Surface Highlight: #3d3d3d (Even Lighter Gray)
- Input Background: #3d3d3d

**Semantic Colors:**
- Success: #4CAF50 (Green)
- Warning: #FF9800 (Orange)
- Error: #F44336 (Red)
- Info: #2196F3 (Blue)

**Text Colors:**
- Primary Text: #ffffff (White)
- Secondary Text: #b0b0b0 (Light Gray)
- Disabled Text: #666666 (Medium Gray)

### Spacing System

**Base Unit:** 4 pixels

**Spacing Scale:**
- xs: 4 pixels (tight spacing)
- sm: 8 pixels (compact spacing)
- md: 16 pixels (standard spacing)
- lg: 24 pixels (generous spacing)
- xl: 32 pixels (large spacing)

---

## Screen 1: Setup Screen

### Layout Structure

The Setup Screen uses a **Border** layout with the following structure:

**Container:** `container.NewBorder(top, bottom, left, right, center)`

**Top Border:** Title label
- Content: "Fabric Mod Bisect Tool"
- Font: 24pt Bold
- Color: #ffffff
- Alignment: Center
- Padding: 16 pixels

**Bottom Border:** Primary action button
- Content: "Start Bisection" button
- Width: 200 pixels minimum
- Height: 40 pixels
- Background: #4CAF50
- Text: #ffffff
- Font: 14pt Semi-Bold
- Importance: High

**Left Border:** Empty (nil)

**Right Border:** Empty (nil)

**Center Content:** Vertical arrangement using VBox
- **Element 1:** Instruction label
  - Content: "Select your mods folder to begin."
  - Font: 14pt Regular
  - Color: #b0b0b0
  - Alignment: Center
  - Spacing: 16 pixels from top

- **Element 2:** Path input card (using Padded layout)
  - Container: `container.NewPadded(inputContainer)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 16 pixels
  - Width: Full available width minus 32 pixels
  
  **Input Container:** `container.NewHBox(entry, browseBtn)`
  - **Entry:** `widget.NewEntry()`
    - Height: 40 pixels
    - Width: 600 pixels (or 70% of available width, minimum 400)
    - Background: #3d3d3d
    - Border: 1px solid #555
    - Border-radius: 4 pixels
    - Placeholder: "Enter or browse for mods folder path..."
    - Placeholder color: #808080
  
  - **Browse Button:** `widget.NewButton("Browse...", func())`
    - Width: 80 pixels
    - Height: 40 pixels
    - Background: #757575
    - Text: #ffffff
    - Font: 14pt Semi-Bold
    - Importance: Medium

- **Element 3:** Help section card (using Padded layout)
  - Container: `container.NewPadded(helpContainer)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 16 pixels
  - Width: Full available width minus 32 pixels
  - Height: 120 pixels (when expanded)
  
  **Help Container:** `container.NewVBox(header, contentItems...)`
  - **Header:** `widget.NewLabel("ℹ️ Tips:")`
    - Font: 14pt Semi-Bold
    - Color: #2196F3
  
  - **Content Items:** Multiple `widget.NewLabel()` widgets
    - Font: 12pt Regular
    - Color: #b0b0b0
    - Spacing: 8 pixels between items
    - Content:
      * "• Ensure your mods folder contains all mods"
      * "• The tool will automatically detect Forge type"
      * "• Bisection may take several minutes"

- **Element 4:** Spacer
  - Type: `layout.NewSpacer()`
  - Purpose: Creates space between help section and bottom button

### Implementation Code Pattern

```go
// Setup Screen using Border layout
content := container.NewBorder(
    titleLabel,                    // Top
    startBtn,                      // Bottom
    nil,                           // Left
    nil,                           // Right
    centerContent,                 // Center
)

// Center content using VBox
centerContent := container.NewVBox(
    instructionLabel,
    pathInputCard,
    helpSectionCard,
    layout.NewSpacer(),
)

// Path input card using Padded layout
pathInputCard := container.NewPadded(container.NewHBox(
    pathEntry,
    browseBtn,
))
```

---

## Screen 2: Loading Screen

### Layout Structure

The Loading Screen uses a **Border** layout with centered content:

**Container:** `container.NewBorder(top, bottom, left, right, center)`

**Top Border:** Empty (nil)

**Bottom Border:** Cancel button
- Content: "Cancel" button
- Width: 100 pixels
- Height: 32 pixels
- Background: #555
- Text: #b0b0b0
- Font: 12pt Regular
- Disabled until 50% progress

**Left Border:** Empty (nil)

**Right Border:** Empty (nil)

**Center Content:** Vertical arrangement using VBox
- **Element 1:** Loading indicator
  - Type: `widget.NewActivity()`
  - Width: 48 pixels
  - Height: 48 pixels
  - Color: #2196F3
  - Animation: Continuous rotation, 1 second per revolution

- **Element 2:** Loading title
  - Content: "Loading..."
  - Font: 24pt Bold
  - Color: #ffffff
  - Alignment: Center

- **Element 3:** Status text
  - Content: Dynamic (e.g., "Initializing bisection engine...")
  - Font: 12pt Regular
  - Color: #b0b0b0
  - Alignment: Center
  - Wrapping: Word wrap

- **Element 4:** Progress bar card (using Padded layout)
  - Container: `container.NewPadded(progressContainer)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 16 pixels
  - Width: 600 pixels (or 75% of available width, minimum 400)
  
  **Progress Container:** `container.NewHBox(progressBar, percentageLabel)`
  - **Progress Bar:** `widget.NewProgressBarInfinite()`
    - Height: 8 pixels
    - Background: #424242
    - Progress color: #4CAF50
    - Border-radius: 4 pixels
  
  - **Percentage Label:** `widget.NewLabel("35%")`
    - Font: 12pt Regular
    - Color: #ffffff
    - Alignment: Right
    - Spacing: 8 pixels from progress bar

- **Element 5:** Info panel card (using Padded layout)
  - Container: `container.NewPadded(infoPanel)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 16 pixels
  - Width: Full available width minus 32 pixels
  
  **Info Panel:** `container.NewVBox(infoItems...)`
  - **Info Items:** Multiple `widget.NewLabel()` widgets
    - Font: 12pt Regular
    - Color: #b0b0b0
    - Spacing: 8 pixels between items
    - Content:
      * "Detected: Fabric + NeoForge"
      * "474 mods loaded"

### Implementation Code Pattern

```go
// Loading Screen using Border layout
content := container.NewBorder(
    nil,                           // Top
    cancelBtn,                     // Bottom
    nil,                           // Left
    nil,                           // Right
    centerContent,                 // Center
)

// Center content using VBox
centerContent := container.NewVBox(
    activityWidget,
    loadingTitle,
    statusLabel,
    progressCard,
    infoPanel,
)

// Progress card using Padded layout
progressCard := container.NewPadded(container.NewHBox(
    progressBar,
    percentageLabel,
))
```

---

## Screen 3: Main Screen (Bisection in Progress)

### Layout Structure

The Main Screen uses a **Border** layout with two main sections:

**Container:** `container.NewBorder(top, bottom, left, right, center)`

**Top Border:** Empty (nil)

**Bottom Border:** Action buttons container
- Container: `container.NewHBox(nextBtn, undoBtn)`
- **Next Step Button:** `widget.NewButton("▶ Next Step", func())`
  - Width: 150 pixels
  - Height: 40 pixels
  - Background: #4CAF50
  - Text: #ffffff
  - Font: 14pt Semi-Bold
  - Importance: High
- **Undo Button:** `widget.NewButton("↩ Undo Last Step", func())`
  - Width: 150 pixels
  - Height: 40 pixels
  - Background: #757575
  - Text: #ffffff
  - Font: 14pt Semi-Bold
  - Importance: Medium
- Spacing: 16 pixels between buttons

**Left Border:** Empty (nil)

**Right Border:** Empty (nil)

**Center Content:** Two-section vertical layout using VBox
- **Section 1:** Progress card (70% of available height)
  - Container: `container.NewPadded(progressCard)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 24 pixels
  - Width: Full available width minus 48 pixels
  - Height: 280 pixels
  
  **Progress Card Content:** `container.NewVBox(title, statusLine, progressCardInner, testPlanCard)`
  
  - **Title:** `widget.NewLabel("BISECTION PROGRESS")`
    - Font: 18pt Semi-Bold
    - Color: #ffffff
    - Alignment: Center
  
  - **Round/Iteration Display:** `widget.NewLabel("Round 3 · Iteration 5")`
    - Font: 24pt Bold
    - Color: #ffffff
    - Alignment: Center
  
  - **Status Line:** `widget.NewLabel("Testing: 12 candidates remaining")`
    - Font: 14pt Regular
    - Color: #b0b0b0
    - Alignment: Center
  
  - **Estimated Steps:** `widget.NewLabel("Estimated: 25 more steps to completion")`
    - Font: 12pt Regular
    - Color: #808080
    - Alignment: Center
  
  - **Progress Bar Card:** `container.NewPadded(progressBarContainer)`
    - Background: #2d2d2d
    - Border: 1px solid #3d3d3d
    - Border-radius: 8 pixels
    - Padding: 16 pixels
    - Width: Full card width minus 32 pixels
  
    **Progress Bar Container:** `container.NewHBox(progressBar, percentageLabel)`
    - **Progress Bar:** `widget.NewProgressBarInfinite()`
      - Height: 8 pixels
      - Background: #424242
      - Progress color: #4CAF50
      - Border-radius: 4 pixels
    - **Percentage Label:** `widget.NewLabel("45%")`
      - Font: 12pt Regular
      - Color: #ffffff
      - Alignment: Right
  
  - **Test Plan Card:** `container.NewPadded(testPlanCard)`
    - Background: #2d2d2d
    - Border: 1px solid #3d3d3d
    - Border-radius: 8 pixels
    - Padding: 16 pixels
    - Width: Full card width minus 32 pixels
    - Height: 120 pixels
  
    **Test Plan Card Content:** `container.NewVBox(header, listItems...)`
    - **Header:** `widget.NewLabel("Current Test Plan:")`
      - Font: 14pt Semi-Bold
      - Color: #2196F3
    - **List Items:** Multiple `widget.NewLabel()` widgets
      - Font: 12pt Regular
      - Color: #b0b0b0
      - Spacing: 8 pixels between items
      - Content:
        * "• Add: accelerateddecay"
        * "• Remove: optiforge"
        * "• Keep: 10 mods unchanged"

- **Section 2:** Test prompt container (30% of available height, collapsible)
  - Container: `container.NewPadded(testPromptCard)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 24 pixels
  - Width: Full available width minus 32 pixels
  - Height: 200 pixels (when expanded)
  - Initial state: Collapsed (height 0)
  
  **Test Prompt Content:** `container.NewVBox(header, instruction, resultGuide, actionButtons)`
  
  - **Header:** `widget.NewLabel("📋 TEST PROTOCOL")`
    - Font: 16pt Semi-Bold
    - Color: #FF9800
    - Alignment: Center
  
  - **Instruction:** `widget.NewLabel(instructionText)`
    - Font: 14pt Regular
    - Color: #ffffff
    - Alignment: Center
    - Wrapping: Word wrap
  
  - **Result Guide Card:** `container.NewPadded(resultGuideCard)`
    - Background: #2d2d2d
    - Border: 1px solid #3d3d3d
    - Border-radius: 8 pixels
    - Padding: 16 pixels
    - Width: Full card width minus 32 pixels
    - Height: 80 pixels
  
    **Result Guide Card Content:** `container.NewVBox(header, guideItems, actionButtons)`
    - **Header:** `widget.NewLabel("Once done, report the result:")`
      - Font: 12pt Semi-Bold
      - Color: #2196F3
      - Alignment: Center
    - **Guide Items:** Multiple `widget.NewLabel()` widgets
      - Font: 12pt Regular
      - Color: #b0b0b0
      - Spacing: 8 pixels between items
      - Content:
        * "• If the game runs fine without the problem → Success"
        * "• If the problem is still there → Failure"
    - **Action Buttons:** `container.NewHBox(successBtn, failureBtn, cancelBtn)`
      - **Success Button:** `widget.NewButton("✔ Success", func())`
        - Width: 120 pixels
        - Height: 40 pixels
        - Background: #4CAF50
        - Text: #ffffff
        - Font: 14pt Semi-Bold
        - Importance: Success
      - **Failure Button:** `widget.NewButton("✖ Failure", func())`
        - Width: 120 pixels
        - Height: 40 pixels
        - Background: #F44336
        - Text: #ffffff
        - Font: 14pt Semi-Bold
        - Importance: Danger
      - **Cancel Button:** `widget.NewButton("Cancel", func())`
        - Width: 100 pixels
        - Height: 40 pixels
        - Background: #555
        - Text: #b0b0b0
        - Font: 12pt Regular
        - Importance: None

### Implementation Code Pattern

```go
// Main Screen using Border layout
content := container.NewBorder(
    nil,                           // Top
    actionButtons,                 // Bottom
    nil,                           // Left
    nil,                           // Right
    centerContent,                 // Center
)

// Center content using VBox
centerContent := container.NewVBox(
    progressCard,
    testPromptCard,
)

// Progress card using Padded layout
progressCard := container.NewPadded(container.NewVBox(
    sectionTitle,
    roundIterationLabel,
    statusLabel,
    estimatedLabel,
    progressBarCard,
    testPlanCard,
))

// Progress bar card using Padded layout
progressBarCard := container.NewPadded(container.NewHBox(
    progressBar,
    percentageLabel,
))
```

---

## Screen 4: Result Screen

### Layout Structure

The Result Screen uses a **Border** layout with multiple card sections:

**Container:** `container.NewBorder(top, bottom, left, right, center)`

**Top Border:** Empty (nil)

**Bottom Border:** Action buttons container
- Container: `container.NewHBox(exportBtn, restartBtn)`
- **Export Results Button:** `widget.NewButton("Export Results", func())`
  - Width: 150 pixels
  - Height: 40 pixels
  - Background: #4CAF50
  - Text: #ffffff
  - Font: 14pt Semi-Bold
  - Importance: High
- **Restart Bisection Button:** `widget.NewButton("Restart Bisection", func())`
  - Width: 150 pixels
  - Height: 40 pixels
  - Background: #757575
  - Text: #ffffff
  - Font: 14pt Semi-Bold
  - Importance: Medium
- Spacing: 16 pixels between buttons

**Left Border:** Empty (nil)

**Right Border:** Empty (nil)

**Center Content:** Vertical arrangement using VBox
- **Element 1:** Title
  - Content: "BISECTION COMPLETE"
  - Font: 24pt Bold
  - Color: #4CAF50
  - Alignment: Center
  - Spacing: 32 pixels from top

- **Element 2:** Summary statistics card (using Padded layout)
  - Container: `container.NewPadded(summaryCard)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 24 pixels
  - Width: Full available width minus 32 pixels
  - Height: 120 pixels
  
  **Summary Card Content:** `container.NewVBox(statsItems...)`
  - **Stats Items:** Multiple `widget.NewLabel()` widgets
    - Font: 12pt Regular
    - Color: #b0b0b0
    - Spacing: 8 pixels between items
    - Content:
      * "Total Tests: 127"
      * "Rounds Completed: 7"
      * "Time Elapsed: 4 minutes 32 seconds"

- **Element 3:** Found conflict section (using Padded layout)
  - Container: `container.NewPadded(foundConflictCard)`
  - Background: #2d2d2d
  - Border: 1px solid #FF5722
  - Border-radius: 8 pixels
  - Padding: 24 pixels
  - Width: Full available width minus 32 pixels
  - Height: 160 pixels
  
  **Found Conflict Card Content:** `container.NewVBox(header, description, conflictCard, suggestionCard)`
  
  - **Header:** `widget.NewLabel("🎯 FOUND CONFLICT")`
    - Font: 16pt Semi-Bold
    - Color: #FF5722
    - Alignment: Center
  
  - **Description:** `widget.NewLabel("The following mod(s) cause your issue:")`
    - Font: 12pt Regular
    - Color: #ffffff
    - Alignment: Center
  
  - **Conflict Card:** `container.NewPadded(conflictCard)`
    - Background: #3d3d3d
    - Border: 1px solid #FF5722
    - Border-radius: 8 pixels
    - Padding: 16 pixels
    - Width: Full card width minus 32 pixels
    - Height: 80 pixels
  
    **Conflict Card Content:** `container.NewVBox(modName, detailsItems...)`
    - **Mod Name:** `widget.NewLabel("accelerateddecay (NeoForge)")`
      - Font: 14pt Semi-Bold
      - Color: #ffffff
    - **Details Items:** Multiple `widget.NewLabel()` widgets
      - Font: 12pt Regular
      - Color: #b0b0b0
      - Spacing: 8 pixels between items
      - Content:
        * "• Added in Round 4"
        * "• Confirmed in 3 verification tests"

  - **Suggestion Card:** `container.NewPadded(suggestionCard)`
    - Background: #2d2d2d
    - Border: 1px solid #2196F3
    - Border-radius: 8 pixels
    - Padding: 16 pixels
    - Width: Full card width minus 32 pixels
    - Height: 60 pixels
  
    **Suggestion Card Content:** `widget.NewLabel("ℹ️ This mod was identified as the root cause. You may want to check for updates or alternatives.")`
      - Font: 12pt Regular
      - Color: #2196F3

- **Element 4:** All conflicts section (using Padded layout)
  - Container: `container.NewPadded(allConflictsCard)`
  - Background: #2d2d2d
  - Border: 1px solid #3d3d3d
  - Border-radius: 8 pixels
  - Padding: 24 pixels
  - Width: Full available width minus 32 pixels
  - Height: 160 pixels (scrollable)
  
  **All Conflicts Card Content:** `container.NewVBox(header, conflictList)`
  
  - **Header:** `widget.NewLabel("📋 ALL DETECTED CONFLICTS")`
    - Font: 16pt Semi-Bold
    - Color: #2196F3
    - Alignment: Center
  
  - **Conflict List:** `widget.NewLabel(conflictListText)`
    - Font: 12pt Regular
    - Color: #b0b0b0
    - Wrapping: Word wrap
    - Content:
      * "Conflict #1: accelerateddecay"
      * "Conflict #2: [none detected]"

- **Element 5:** Cleared mods section (using Padded layout)
  - Container: `container.NewPadded(clearedModsCard)`
  - Background: #2d2d2d
  - Border: 1px solid #4CAF50
  - Border-radius: 8 pixels
  - Padding: 24 pixels
  - Width: Full available width minus 32 pixels
  - Height: 120 pixels
  
  **Cleared Mods Card Content:** `container.NewVBox(header, description, modList)`
  
  - **Header:** `widget.NewLabel("✅ CLEARED MODS")`
    - Font: 16pt Semi-Bold
    - Color: #4CAF50
    - Alignment: Center
  
  - **Description:** `widget.NewLabel("These mods were removed and verified safe:")`
    - Font: 12pt Regular
    - Color: #ffffff
    - Alignment: Center
  
  - **Mod List:** `widget.NewLabel(modListText)`
    - Font: 12pt Regular
    - Color: #b0b0b0
    - Wrapping: Word wrap
    - Content:
      * "• optiforge"
      * "• fabric-api"
      * "• ... (12 more)"

### Implementation Code Pattern

```go
// Result Screen using Border layout
content := container.NewBorder(
    nil,                           // Top
    actionButtons,                 // Bottom
    nil,                           // Left
    nil,                           // Right
    centerContent,                 // Center
)

// Center content using VBox
centerContent := container.NewVBox(
    titleLabel,
    summaryCard,
    foundConflictCard,
    allConflictsCard,
    clearedModsCard,
)

// Summary card using Padded layout
summaryCard := container.NewPadded(container.NewVBox(
    stat1,
    stat2,
    stat3,
))

// Found conflict card using Padded layout
foundConflictCard := container.NewPadded(container.NewVBox(
    header,
    description,
    conflictCard,
    suggestionCard,
))
```

---

## Component Library

### Buttons

**Primary Button:**
- Width: 150-200 pixels
- Height: 40 pixels
- Padding: 8 pixels vertical × 16 pixels horizontal
- Background: #4CAF50
- Text color: #ffffff
- Font: 14pt Semi-Bold
- Border-radius: 4 pixels
- Importance: High
- Hover: #45a049
- Active: #3d8b40
- Disabled: Background #555, Text #666666

**Secondary Button:**
- Width: 100-150 pixels
- Height: 40 pixels
- Padding: 8 pixels vertical × 16 pixels horizontal
- Background: #757575
- Text color: #ffffff
- Font: 14pt Semi-Bold
- Border-radius: 4 pixels
- Importance: Medium
- Hover: #616161
- Disabled: Background #555, Text #666666

**Tertiary Button:**
- Width: 80-120 pixels
- Height: 40 pixels
- Padding: 8 pixels vertical × 16 pixels horizontal
- Background: Transparent
- Border: 1px solid #555
- Text color: #b0b0b0
- Font: 12pt Regular
- Border-radius: 4 pixels
- Importance: None
- Hover: Background #333

**Danger Button:**
- Width: 100-120 pixels
- Height: 40 pixels
- Padding: 8 pixels vertical × 16 pixels horizontal
- Background: #F44336
- Text color: #ffffff
- Font: 14pt Semi-Bold
- Border-radius: 4 pixels
- Importance: Danger
- Hover: #d32f2f

**Success Button:**
- Width: 100-120 pixels
- Height: 40 pixels
- Padding: 8 pixels vertical × 16 pixels horizontal
- Background: #4CAF50
- Text color: #ffffff
- Font: 14pt Semi-Bold
- Border-radius: 4 pixels
- Importance: Success
- Hover: #45a049

### Progress Bar

- Height: 8 pixels
- Background: #424242
- Progress color: Variable (semantic color)
- Border-radius: 4 pixels
- Width: 600 pixels or 75% of available width
- Percentage label: 12pt Regular, #ffffff, right-aligned, 8 pixels from bar

### Cards

- Background: #2d2d2d
- Border: 1px solid #3d3d3d
- Border-radius: 8 pixels
- Padding: 16 pixels
- Shadow: 0 2 4 rgba(0,0,0,0.3)

### Input Fields

- Height: 40 pixels
- Width: 600 pixels or 70% of available width
- Background: #3d3d3d
- Border: 1px solid #555
- Border-radius: 4 pixels
- Text color: #ffffff
- Placeholder color: #808080
- Focus border: #2196F3

### Accordions

- Header height: 40 pixels
- Header background: #2d2d2d
- Header border: 1px solid #3d3d3d
- Header border-radius: 4 pixels
- Header font: 14pt Semi-Bold
- Header color: #2196F3
- Content font: 12pt Regular
- Content color: #b0b0b0
- Content spacing: 8 pixels between items

---

## Layout Usage Summary

### Primary Layouts Used

| Screen  | Primary Layout | Secondary Layouts  |
| ------- | -------------- | ------------------ |
| Setup   | Border         | VBox, Padded, HBox |
| Loading | Border         | VBox, Padded, HBox |
| Main    | Border         | VBox, Padded, HBox |
| Result  | Border         | VBox, Padded, HBox |

### Layout Combination Patterns

**Pattern 1: Border with VBox Center**
```go
container.NewBorder(
    topElement,
    bottomElement,
    nil,
    nil,
    container.NewVBox(centerElements...),
)
```

**Pattern 2: Padded with HBox/VBox**
```go
container.NewPadded(container.NewHBox/VBox(elements...))
```

**Pattern 3: Nested Padded**
```go
container.NewPadded(container.NewPadded(container.NewHBox/VBox(elements...)))
```

**Pattern 4: Border with Multiple Center Elements**
```go
container.NewBorder(
    topElement,
    bottomElement,
    nil,
    nil,
    container.NewVBox(
        element1,
        element2,
        element3,
        layout.NewSpacer(),
        element4,
    ),
)
```

---

## Interaction Patterns

### Screen Transitions

**Fade + Slide Animation:**
1. New screen fades in over 300ms
2. Previous screen fades out over 300ms
3. Total transition duration: 600ms
4. Loading spinner visible during transition

**Loading Indicator:**
- Spinner appears 200ms before screen change
- Fades out 200ms after new screen is ready

### Button States

| State    | Description       | Visual                       |
| -------- | ----------------- | ---------------------------- |
| Default  | Normal state      | Standard colors              |
| Hover    | Mouse cursor over | Darker background            |
| Active   | Mouse clicked     | Even darker background       |
| Disabled | Not interactive   | Grayed out, no hover effect  |
| Loading  | Processing action | Spinner icon, disabled state |

### Test Prompt Behavior

- Appears as inline expansion below main content
- Expand animation: 200ms ease-in-out
- Collapse animation: 200ms ease-in-out
- Does not block main interface
- Can be toggled open/closed

---

## Responsive Design

### Window Size Recommendations

**Minimum:** 600 × 400 pixels
**Recommended:** 800 × 600 pixels
**Optimal:** 1024 × 768 pixels or larger

### Breakpoint Behavior

| Window Width   | Layout Behavior                                           |
| -------------- | --------------------------------------------------------- |
| < 600 pixels   | Single column, all elements stacked vertically            |
| 600-800 pixels | Two columns where appropriate, buttons side-by-side       |
| > 800 pixels   | Full layout with proper spacing and side-by-side elements |

### Adaptive Spacing

- Reduce padding by 20% on smaller windows
- Reduce font sizes by 1pt on windows < 700 pixels wide
- Stack side-by-side buttons vertically on windows < 650 pixels wide

---

## Accessibility

### WCAG 2.1 AA Compliance

**Color Contrast:**
- Normal text on background: Minimum 4.5:1 contrast ratio
- Large text (18pt+ or 14pt+ bold): Minimum 3:1 contrast ratio
- All proposed colors meet these requirements

**Focus Indicators:**
- Visible focus ring: 2 pixels, #2196F3 color
- Focus visible on all interactive elements
- Focus outline does not obscure content

**Keyboard Navigation:**
- Tab order follows visual layout (top-to-bottom, left-to-right)
- Escape key closes dialogs and modals
- Arrow keys navigate lists and menus
- Enter/Space activates buttons

---

## Implementation Notes

### Fyne Layout Best Practices

1. **Border Layout:** Use for main screen structure with top/bottom/left/right/center positioning
2. **VBox:** Use for vertical stacking of elements with equal width
3. **HBox:** Use for horizontal arrangement of elements with equal height
4. **Padded:** Use to add consistent padding around content
5. **Spacer:** Use to create flexible spacing between elements
6. **Center:** Use for centering content in available space

### Widget Selection Guidelines

- Use `widget.NewLabel()` for static text
- Use `widget.NewLabelWithStyle()` for styled text (bold, alignment)
- Use `widget.NewButton()` for buttons with text
- Use `widget.NewEntry()` for text input
- Use `widget.NewProgressBarInfinite()` for loading indicators
- Use `widget.NewSeparator()` for visual dividers
- Use `widget.NewActivity()` for animated loading spinners

### Data Binding

Use data binding for dynamic content updates:

```go
// Example: Binding progress percentage
progressPercent := binding.NewFloat()
percentageLabel := widget.NewLabelWithData(binding.Float(progressPercent))

// Update in goroutine
go func() {
    progressPercent.Set(45.5)
}()
```

---

*Document Version: 3.0*
*Last Updated: 2024*
*Author: UI/UX Design Team*
