---
version: alpha
name: Ocean
description: Tema corporativo frío, alto contraste.
colors:
  primary: "#2563eb"
  secondary: "#7c3aed"
  tertiary: "#06b6d4"
  neutral: "#ffffff"
  surface: "#f3f4f6"
  on-surface: "#111827"
  info: "#0ea5e9"
  success: "#16a34a"
  warning: "#d97706"
  error: "#dc2626"
typography:
  body-md:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.5
  label-md:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 600
  code-md:
    fontFamily: "JetBrains Mono, monospace"
    fontSize: "14px"
rounded:
  sm: "0.5rem"
  md: "1rem"
spacing:
  xs: "0.25rem"
  sm: "0.5rem"
  md: "1rem"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#ffffff"
    rounded: "{rounded.sm}"
    padding: "{spacing.sm}"
x-pi:
  themeId: ocean
  colorScheme: light
  daisyui:
    primary-content: "#ffffff"
    accent: "{colors.tertiary}"
    base-100: "{colors.neutral}"
    base-200: "{colors.surface}"
    base-300: "#e5e7eb"
    base-content: "{colors.on-surface}"
    neutral: "#1f2937"
    neutral-content: "#ffffff"
    radius-box: "{rounded.md}"
    radius-field: "{rounded.sm}"
    radius-selector: "{rounded.sm}"
    shadow-card: "0 22px 56px -30px color-mix(in srgb, #1f2937 24%, transparent)"
    shadow-card-soft: "0 12px 30px -20px color-mix(in srgb, #1f2937 16%, transparent)"
    shadow-sidebar: "0 28px 64px -34px color-mix(in srgb, #1f2937 28%, transparent)"
    border-subtle: "color-mix(in srgb, #111827 10%, transparent)"
    surface-muted: "color-mix(in srgb, #f3f4f6 86%, white)"
    surface-elevated: "color-mix(in srgb, #ffffff 95%, white)"
---

# Ocean

## Overview

Tema corporativo frío, sobrio y de alto contraste, orientado a productividad.

## Colors

El azul principal comunica confianza y acción. El fondo se mantiene claro para
favorecer lectura y jerarquía de contenido.

## Typography

Inter gobierna la interfaz y JetBrains Mono se reserva para datos técnicos y
fragmentos de código.
