# Schema del perfil `DESIGN.md` del proyecto

Este proyecto adopta `DESIGN.md` alineado al formato de Google y usa una capa
mínima de extensiones `x-pi` para runtime theming con DaisyUI v5.

## Campos canónicos soportados en MVP

- `version`
- `name`
- `description`
- `colors`
- `typography`
- `rounded`
- `spacing`
- `components`

## Extensiones `x-pi`

```yaml
x-pi:
  themeId: <string>         # opcional; si no existe se deriva desde la carpeta
  colorScheme: <string>     # opcional; light | dark
  daisyui:
    <token-name>: <string | token reference>
```

## Extensiones `x-pi.daisyui` reconocidas inicialmente

- `primary-content`
- `accent`
- `base-100`
- `base-200`
- `base-300`
- `base-content`
- `neutral`
- `neutral-content`
- `radius-box`
- `radius-field`
- `radius-selector`

## Reglas del MVP

- Los tokens estándar son la fuente principal.
- `x-pi.daisyui` solo debe usarse cuando DaisyUI necesita aliases o defaults que
  no se puedan inferir bien desde el spec base.
- Un tema inválido no debe tumbar el server; se excluye del catálogo.
