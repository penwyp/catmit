# Unified TUI Framework Comparison

## Overview
This document compares the original TUI implementations with the new unified framework approach.

## Benefits of the Unified Framework

### 1. Consistent User Experience
- **All TUIs now support:**
  - Esc key for cancellation
  - Arrow keys for navigation (←/→/↑/↓)
  - Enter for confirmation
  - Letter shortcuts (A/E/C/Q etc.)
  - Wrapping navigation (first to last, last to first)

### 2. Code Reduction
- **Original ReviewModel:** ~314 lines
- **SimpleReviewModel:** ~185 lines
- **Reduction:** ~40% less code

### 3. Simplified Implementation

#### Before (Original SquashModel keyboard handling):
```go
case tea.KeyMsg:
    switch m.phase {
    case SquashPhaseReviewing:
        switch msg.String() {
        case "a", "A":
            // Accept logic
        case "r", "R":
            // Regenerate logic
        case "e", "E":
            // Edit logic
        case "q", "Q", "ctrl+c":
            // Quit logic
        }
    }
```

#### After (Using BaseModel):
```go
case tea.KeyMsg:
    if m.phase == SquashPhaseReviewing {
        cmd := m.HandleKeyboard(msg)
        if cmd != nil {
            return m, cmd
        }
    }
```

### 4. Visual Consistency
All models now use the same clean, padding-based style inspired by the squash model:
- No heavy borders
- Clean typography
- Consistent color scheme
- Professional appearance

### 5. New Features
- **Append Mode:** Non-clearing console output for better history
- **Visual Selection:** Selected actions are highlighted
- **Flexible Actions:** Easy to add/remove actions dynamically

### 6. Better Maintainability
- Single place to update keyboard handling logic
- Consistent styling through UIStyles
- Reusable components and utilities
- Clear separation of concerns

## Migration Guide

### Step 1: Embed BaseModel
```go
type MyModel struct {
    BaseModel
    // your fields
}
```

### Step 2: Initialize with BaseModel
```go
model := &MyModel{
    BaseModel: NewBaseModel("Title", actions, appendMode),
    // your initialization
}
model.SetContentRenderer(model.renderContent)
```

### Step 3: Define Actions
```go
actions := []Action{
    {Key: "A", Label: "ccept", Handler: model.accept},
    {Key: "C", Label: "ancel", Handler: model.cancel},
}
```

### Step 4: Implement Content Renderer
```go
func (m *MyModel) renderContent() string {
    // Return your main content
}
```

### Step 5: Use BaseModel's Update
```go
func (m *MyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        cmd := m.HandleKeyboard(msg)
        if cmd != nil {
            return m, cmd
        }
    }
    // Handle other messages
}
```

## Conclusion
The unified framework provides a consistent, maintainable, and user-friendly approach to building TUIs in the catmit project. It reduces code duplication, improves user experience, and makes it easier to add new TUI-based features in the future.