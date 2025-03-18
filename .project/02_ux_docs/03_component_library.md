# UI Component Library

This document defines the standard UI components used throughout the Talent Rodeo application. It serves as a reference for maintaining consistency across the user interface.

## Design System Overview

Talent Rodeo uses a lightweight design system based on PicoCSS with custom extensions. The design prioritizes:
- Clean, minimalist aesthetics
- Strong readability
- Accessible color contrasts
- Intuitive interaction patterns
- Responsive behavior

## Core Components

### 1. Buttons

#### Primary Button
- Used for main actions on a page
- High visual prominence
- Example usage: "Start Search", "Schedule Call", "Save Preferences"
- Implementation: `<button class="primary">Label</button>`

#### Secondary Button
- Used for alternative actions
- Medium visual prominence
- Example usage: "Cancel", "Back", "Reset"
- Implementation: `<button class="secondary">Label</button>`

#### Text Button
- Used for tertiary actions
- Low visual prominence
- Example usage: "Learn more", "Show details"
- Implementation: `<button class="text">Label</button>`

#### Icon Button
- Used for compact UI actions
- Contains icon with optional tooltip
- Example usage: Edit, Delete, Refresh actions
- Implementation: `<button class="icon" aria-label="Description"><i class="icon-name"></i></button>`

### 2. Form Elements

#### Text Input
- Standard text entry field
- Includes floating label pattern
- Validation state indicators
- Implementation: 
  ```html
  <div class="field">
    <label for="fieldId">Label</label>
    <input type="text" id="fieldId" name="fieldName" required>
    <small class="error">Error message</small>
  </div>
  ```

#### Select Dropdown
- Used for selecting from predefined options
- Custom styling for consistent cross-browser appearance
- Implementation:
  ```html
  <div class="field">
    <label for="selectId">Label</label>
    <select id="selectId" name="selectName">
      <option value="option1">Option 1</option>
      <option value="option2">Option 2</option>
    </select>
  </div>
  ```

#### Checkbox
- Used for boolean selections or multi-select lists
- Custom styling for improved usability
- Implementation:
  ```html
  <label class="checkbox">
    <input type="checkbox" name="checkboxName" value="value">
    <span>Checkbox Label</span>
  </label>
  ```

#### Radio Button
- Used for single selection from options
- Custom styling for improved usability
- Implementation:
  ```html
  <label class="radio">
    <input type="radio" name="radioName" value="value">
    <span>Radio Label</span>
  </label>
  ```

#### File Upload
- Used for resume upload
- Supports drag-and-drop and click selection
- Visual feedback during upload
- Implementation:
  ```html
  <div class="file-upload" data-hx-target="#uploadResult">
    <label for="fileInput">Upload File</label>
    <input type="file" id="fileInput" name="fileName" accept=".pdf,.docx">
    <div class="upload-zone">
      <p>Drag files here or click to browse</p>
    </div>
    <div id="uploadResult"></div>
  </div>
  ```

### 3. Containers

#### Card
- Used for containing related content
- Supports header, content, and footer sections
- Implementation:
  ```html
  <article class="card">
    <header>Card Title</header>
    <div>Card content goes here</div>
    <footer>Card footer content</footer>
  </article>
  ```

#### Panel
- Used for sectioning content on a page
- Less prominent than cards
- Implementation: `<section class="panel">Panel content</section>`

#### Modal
- Used for focused interactions
- Blocks interaction with page underneath
- Implementation:
  ```html
  <dialog id="modalId">
    <article>
      <header>
        <h3>Modal Title</h3>
        <button class="close" aria-label="Close">×</button>
      </header>
      <div>Modal content</div>
      <footer>Modal footer with actions</footer>
    </article>
  </dialog>
  ```

#### Toast Notification
- Used for temporary feedback
- Auto-dismisses after timeout
- Implementation:
  ```html
  <div class="toast" role="alert" data-level="info">
    <div class="toast-content">
      <i class="toast-icon"></i>
      <span>Notification message</span>
    </div>
  </div>
  ```

### 4. Navigation

#### Primary Navigation
- Main site navigation
- Adapts to mobile with hamburger menu
- Implementation:
  ```html
  <nav>
    <ul>
      <li><a href="/">Home</a></li>
      <li><a href="/profile">Profile</a></li>
      <!-- More items -->
    </ul>
  </nav>
  ```

#### Breadcrumbs
- Secondary navigation showing location in site hierarchy
- Implementation:
  ```html
  <nav aria-label="Breadcrumb">
    <ul>
      <li><a href="/">Home</a></li>
      <li><a href="/profile">Profile</a></li>
      <li>Current Page</li>
    </ul>
  </nav>
  ```

#### Tabs
- Content organization within a page
- Implementation:
  ```html
  <div class="tabs">
    <nav>
      <a href="#tab1" class="active">Tab 1</a>
      <a href="#tab2">Tab 2</a>
    </nav>
    <div id="tab1" class="tab-content active">Tab 1 content</div>
    <div id="tab2" class="tab-content">Tab 2 content</div>
  </div>
  ```

### 5. Data Display

#### Table
- Used for structured data display
- Supports sorting and pagination
- Responsive behavior on small screens
- Implementation:
  ```html
  <table>
    <thead>
      <tr>
        <th>Column 1</th>
        <th>Column 2</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>Data 1</td>
        <td>Data 2</td>
      </tr>
    </tbody>
  </table>
  ```

#### List
- Used for displaying collections of similar items
- Implementation:
  ```html
  <ul class="item-list">
    <li class="item">
      <div class="item-content">Item content</div>
      <div class="item-actions">Item actions</div>
    </li>
  </ul>
  ```

#### Progress Bar
- Used for showing completion or loading progress
- Implementation:
  ```html
  <div class="progress-container">
    <progress value="75" max="100"></progress>
    <span>75%</span>
  </div>
  ```

#### Badge
- Used for status indicators and counts
- Implementation: `<span class="badge badge-primary">New</span>`

### 6. Special Components

#### Calendar
- Used for appointment scheduling
- Supports date selection and time slots
- Implementation based on vanilla JavaScript
- Implementation:
  ```html
  <div class="calendar" data-hx-get="/api/slots" data-hx-trigger="calendar:dateSelect">
    <div class="calendar-header">
      <button class="prev-month">&lt;</button>
      <span class="current-month">May 2023</span>
      <button class="next-month">&gt;</button>
    </div>
    <div class="calendar-grid">
      <!-- Calendar days -->
    </div>
    <div class="time-slots">
      <!-- Time slots loaded via HTMX -->
    </div>
  </div>
  ```

#### Skills List
- Used for displaying and managing skills
- Supports adding, editing, and removing skills
- Implementation:
  ```html
  <div class="skills-container">
    <ul class="skills-list">
      <li class="skill-item">
        <span class="skill-name">Skill Name</span>
        <div class="skill-actions">
          <button class="edit-skill">Edit</button>
          <button class="delete-skill">Delete</button>
        </div>
      </li>
    </ul>
    <form class="add-skill-form">
      <input type="text" name="skillName" placeholder="Add new skill">
      <button type="submit">Add</button>
    </form>
  </div>
  ```

#### Job Card
- Used for displaying job listings
- Shows key job information and match metrics
- Implementation:
  ```html
  <article class="job-card">
    <header>
      <h3 class="job-title">Job Title</h3>
      <span class="match-score">85%</span>
    </header>
    <div class="job-details">
      <div class="company">Company Name</div>
      <div class="location">Location</div>
      <div class="salary">Salary Range</div>
      <div class="posted-date">Posted 2 days ago</div>
    </div>
    <footer>
      <button class="view-details">View Details</button>
    </footer>
  </article>
  ```

#### Audio Recorder
- Used for interview practice
- Provides recording controls and feedback
- Implementation:
  ```html
  <div class="audio-recorder">
    <button class="record-button">Record</button>
    <button class="stop-button" disabled>Stop</button>
    <div class="recording-indicator">
      <span class="timer">00:00</span>
      <div class="waveform"></div>
    </div>
    <audio class="playback" controls></audio>
  </div>
  ```

## Visual Style

### Typography

- **Primary Font:** System font stack for optimal performance
- **Heading Scales:**
  - H1: 2rem (32px)
  - H2: 1.5rem (24px)
  - H3: 1.25rem (20px)
  - H4: 1rem (16px)
- **Body Text:** 1rem (16px)
- **Small Text:** 0.875rem (14px)

### Color Palette

- **Primary:** #4361EE - Used for primary actions and key elements
- **Secondary:** #3A0CA3 - Used for secondary elements and accents
- **Success:** #4CC9F0 - Used for success states and positive feedback
- **Warning:** #F72585 - Used for warnings and attention states
- **Error:** #F94144 - Used for error states and critical information
- **Neutral:** 
  - Dark: #2B2D42 - Text and high-contrast elements
  - Medium: #8D99AE - Secondary text and borders
  - Light: #EDF2F4 - Backgrounds and low-contrast elements

### Spacing System

- **Base Unit:** 0.25rem (4px)
- **Scale:**
  - xs: 0.25rem (4px)
  - sm: 0.5rem (8px)
  - md: 1rem (16px)
  - lg: 1.5rem (24px)
  - xl: 2rem (32px)
  - xxl: 3rem (48px)

### Responsive Breakpoints

- **Mobile:** < 576px
- **Tablet:** 576px - 992px
- **Desktop:** > 992px

## Implementation Guidelines

### Template Structure

All components should follow the established template pattern:
- HTML structure in `assets/templates/partials/`
- Self-contained CSS in component file or appropriate stylesheet
- Any JavaScript behaviors isolated to the component

### Accessibility Requirements

All components must meet these accessibility requirements:
- Keyboard navigable
- Screen reader friendly with appropriate ARIA attributes
- Sufficient color contrast (WCAG AA minimum)
- Focus states clearly visible
- Proper heading hierarchy
- Responsive to text size changes

### Documentation Format

When adding new components, follow this documentation format:
- Component name and purpose
- Implementation example
- Variant examples
- Accessibility considerations
- Usage guidelines

### Dynamic Content

For dynamic content updates:
- Use HTMX attributes for server interactions
- Use Alpine.js directives for client-side state management
- Ensure graceful degradation without JavaScript
- Provide appropriate loading states

## Template Examples

Complete examples of key templates can be found in:
- `assets/templates/pages/` - Full page templates
- `assets/templates/partials/` - Reusable component templates

## CSS Organization

The CSS follows a component-based organization:
- Base styles in core stylesheet
- Component-specific styles defined with component
- Utility classes for common patterns
- Minimal use of global styles 