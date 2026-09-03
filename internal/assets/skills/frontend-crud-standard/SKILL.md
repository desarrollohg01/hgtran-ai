---
name: frontend-crud-standard
description: "Trigger: pantalla Angular, tabla, filtros, formulario de edicion, Angular Material, estado vacio. Estandares de frontend de HG."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
---

## Cuándo usarla

Al construir o revisar una pantalla de catálogo en Angular con Angular Material: tablas, filtros,
formularios de alta y edición, estados vacíos.

**No todas las webapps de HG usan Angular Material.** Antes de aplicar nada, mirá las PLANTILLAS
del repositorio: la dependencia del `package.json` no alcanza — `operationsboard-webapp` declara
`@angular/material` y no tiene una sola plantilla con componentes de Material.

- Angular Material en las plantillas: `portaltools-webapp` y `hgproveedorextranet-webapp`. Ahí
  aplica esta skill entera.
- PrimeNG: `operationsboard-webapp`, `comedor/frontend`, `ediboard-webapp`. Ahí la sección de
  Angular Material no aplica. Y sobre `operationsboard-webapp` manda `web-ui-standard`, que exige
  PrimeNG o los componentes compartidos y el esqueleto de pantalla (`placeholder-title`,
  `placeholder-content`, `app-loader`, `p-toast`). Donde esa spec gobierna, gana ella.

Lo que sí es transversal: fechas, estados vacíos, formularios de edición, consumo de la API,
dependencias y verificación.

Cada regla nació de un defecto real. Varias causaron pérdida de datos.

## La tabla es compartida

Una pantalla declara QUÉ mostrar; paginar, ordenar y filtrar es de la tabla.

**El contrato no es el mismo en todos los repositorios.** `ColumnConfig` es el único nombre común:
`TableAction`, `ParametrosGenerales` y `fetchDataFunction` son de las webapps con Material y no
existen en `operationsboard-webapp`; `filterKey` y `filterType: 'autocomplete'` hoy sólo viven en
`hgproveedorextranet-webapp`. Leé el contrato real de la tabla del repositorio antes de escribir
una columna.

- Contrato de referencia: `ColumnConfig`, `TableAction`, `ParametrosGenerales`, `fetchDataFunction`.
- `customRender` recibe la **FILA COMPLETA**, no el valor de la celda.
- `filterKey` separa lo que se MUESTRA de lo que se FILTRA: la columna pinta el nombre del
  proveedor y filtra por su identificador, que es exacto y no depende de acentos.
- Catálogo grande (cientos o más) → `filterType: 'autocomplete'` con fuente paginada, NUNCA un
  `select`: abrir un menú no puede costar descargar 1 766 registros.
- Si la tabla del repositorio no expone la pieza equivalente, la regla se traduce al contrato que
  sí tiene; no se inventa una API que no existe ahí.

## El sobre de la API

El backend responde `Response<T>`, y **un fallo de negocio llega como HTTP 200 con
`Success = false`** — lo fija `backend-crud-standard`. Consumirlo asumiendo "200 = salió bien"
pinta éxito sobre un rechazo, o una tabla vacía sobre un error.

- REQUIERE inspeccionar `Success` antes de usar el cuerpo. El veredicto está en `Success`, el motivo
  en `ErrorList`, los datos en `Items` (colección) o `Item` (uno), y el universo de la búsqueda en
  `TotalRecords` — no en el largo de `Items`, que es sólo la página.
- **RECHAZA `catchError(() => of([]))`** y su forma equivalente en bloque. Descarta `ErrorList` y la
  pantalla queda sin poder decir qué pasó: el operador lee "Aún no hay registros" donde lo que hubo
  fue un fallo.
- El patrón que exige `api-crud-standard` es `ensureSuccessResponse` / `normalizeApiError` en el
  servicio DataAccess. Es OBJETIVO para servicios nuevos y para los que se tocan; la base todavía
  arrastra el anti-patrón en muchos archivos y migra oportunistamente.

## Formularios de edición

**RECHAZA un formulario de edición que abra vacío.** Si un campo es obligatorio y llega en blanco,
el operador tiene que reescribirlo, y si el `PUT` del servicio reemplaza en vez de mezclar, lo que
teclee sustituye al valor real. Ocurrió: se perdieron nombres irrecuperables.

- REQUIERE precargar con los datos vigentes antes de habilitar la edición.
- Mientras cargan, los campos van DESHABILITADOS. Si no llegan, se BLOQUEA el guardado con una
  explicación.
- **`form.value` OMITE los controles deshabilitados.** Usa `getRawValue()`, o el `PUT` viaja con
  `undefined` en esos campos — que en un contrato de reemplazo equivale a borrarlos.

## Fechas

**Nunca `toISOString()` para serializar la fecha de un filtro.** Convierte a UTC y el día del
calendario se corre: al este de Greenwich la medianoche local ya es el día anterior; al oeste
(México) cualquier hora desde las 18:00 cae en el día siguiente. Se leen los componentes LOCALES.

## Estados vacíos

Son DOS situaciones distintas y merecen mensajes distintos:

| Situación | Mensaje | Acción |
|---|---|---|
| Catálogo vacío | "Aún no hay registros." | ninguna |
| Filtros sin coincidencias | "Ningún registro coincide con los filtros." | botón para limpiarlos |

Decir "aún no hay registros" con un filtro puesto es FALSO y manda a buscar el problema donde no
está: uno revisa si el alta falló cuando lo único que sobra es un filtro.

El estado vacío va centrado en el ÁREA DE DATOS, no en la pantalla —centrarlo en la pantalla lo
pone sobre el encabezado—. Pegado arriba con el vacío debajo se lee como si algo no hubiera
cargado.

## Angular Material: contratos que no son obvios

| Trampa | Realidad |
|---|---|
| `panelClass` en `<mat-autocomplete>` | NO llega al panel. Va al contenedor del overlay. Se usa el input **`class`** |
| `mat-sort-header` | Va en el `<th>`, que es el uso documentado. NO renderiza un `<button>`: es un `<div role="button">` con `tabindex`, elegido a propósito para que el lector de pantalla anuncie el `aria-sort` del encabezado |
| `aria-sort` | Lo pone el propio `MatSortHeader` como host binding sobre el elemento que lleva la directiva. Moverla a un `<span>` interno deja el `aria-sort` en un `span`, donde es inerte: sólo tiene efecto en `columnheader`/`rowheader` |
| Paginador en inglés | Sólo se traduce con `MatPaginatorIntl`; no se puede desde la plantilla |
| Overlays transparentes | Faltan los tokens del tema: sin tema, las variables `--mat-*` quedan vacías. Es la causa raíz, no se parchea panel por panel. **El mixin depende de la versión**: `mat.theme()` existe desde Material 19; en 16 y 18 el equivalente es `mat.all-component-themes($tema)` con `mat.define-light-theme` / `define-dark-theme` |

## Tablas: `table-layout: fixed`

- Es lo que hace que los anchos declarados se cumplan. Con `auto`, el contenido más terco decide.
- Con `fixed`, un ancho corto RECORTA en vez de ensanchar: hay que decir qué hacer con el texto que
  no cabe.
- **El ancho dibujado NO es el declarado**: el navegador reparte el sobrante. Guardar el dibujado
  como declarado hace que la columna crezca sola en cada interacción.
- Una columna de RELLENO sin ancho declarado absorbe el sobrante: así las demás conservan su medida
  y la de acciones queda pegada al margen derecho. Debe existir SIEMPRE (midiendo cero cuando no
  hace falta): agregarla o quitarla cambia el conjunto de columnas y `mat-table` reconstruye el
  encabezado, destruyendo cualquier interacción en curso.
- Un `<td>` con `display: flex` deja de ser celda y el navegador IGNORA su `colspan`.

## Estilo

- RECHAZA cualquier hex en un componente: el color sale del token, y el tema oscuro depende de eso.
  `var(--hg-*)` es la nomenclatura de `hgproveedorextranet-webapp`; `portaltools-webapp` no la usa.
  Averiguá el prefijo real del repositorio antes de escribir una variable.
- Una sola familia tipográfica, por token. Material NO hereda la fuente del documento: hay que
  pasársela al tema. Los controles de formulario tampoco: necesitan `font-family: inherit`.
- Fuente por npm, nunca por CDN: es una aplicación de intranet. Donde manda `web-ui-standard` la
  regla es más fuerte todavía y va en el otro sentido — pila tipográfica de SISTEMA, sin depender de
  webfonts descargadas. Ninguna de las dos admite el CDN.

## Dependencias

- Antes de instalar, consultar advisories. `xlsx` (SheetJS) sólo publica en npm la 0.18.5, con dos
  advisories de severidad alta; las corregidas están en su CDN propio. Se usa `exceljs`.
- Librerías pesadas con `import()` DINÁMICO. Estático, exceljs sumaba 1.8 MB al bundle de cada
  pantalla, lo pagara o no quien exporta.

## Verificación

- `document.fonts.check()` da FALSOS POSITIVOS. `getComputedStyle` dice lo que la CSS declara, no
  lo que el navegador hace. Para afirmar que algo se ve: mirar la red y MEDIR.
- **El dev server de Angular sirve bundles viejos** tras un error de compilación intermedio, y no
  vigila `angular.json`. Ante cualquier rareza, `ng build` desde cero para separar "código roto" de
  "servidor desactualizado", y reiniciarlo.
- Un backdrop de overlay abierto intercepta el cursor y hace parecer que un hover no funciona.
  `document.elementsFromPoint` muestra la pila real.
