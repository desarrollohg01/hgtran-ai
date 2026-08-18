---
name: real-system-verification
description: "Trigger: verificar, probar, validar, dar por terminado, las pruebas pasan. Disciplina de comprobar contra sistemas reales antes de afirmar que sirve."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
---

## Cuándo usarla

Antes de afirmar que algo funciona. En especial cuando las pruebas están en verde: ese es
justamente el momento de mayor riesgo, porque invita a cerrar.

## La regla

Las pruebas verdes dicen que el código hace lo que ALGUIEN SUPUSO. Sólo el sistema real dice si esa
suposición era cierta.

Un doble reproduce la suposición de quien lo escribió. Si la suposición está mal, el doble la
confirma y la prueba pasa igual.

## Casos reales de esta regla

Cada uno pasó desapercibido con la suite en verde:

| Lo que decía la herramienta | Lo que pasaba de verdad |
|---|---|
| 64 pruebas en verde sobre la integración | El claim que leía NO EXISTÍA, el validador rechazaba todo token legítimo y el cliente nunca enviaba el token. Los tres dobles compartían la misma suposición |
| `document.fonts.check()` → `true` | La fuente jamás se descargó. Falso positivo |
| `getComputedStyle` → la familia correcta | Dice lo que la CSS DECLARA, no lo que el navegador dibuja |
| El código sugería que `[Authorize]` bastaba | `PUT` con el token de otro tenant devolvió **200** |
| Ancho declarado 10 % vs 18 % | Ambas medían 244 px: el contenido imponía el ancho, no la declaración |
| Build correcto | El dev server servía un bundle viejo de un error intermedio |
| El JSON "se veía bien" | La misma propiedad salía con `Z` al crear y sin `Z` al leer: seis horas de corrimiento |

## Qué hacer

| En vez de | Hacer |
|---|---|
| Confiar en el doble | Ejecutar contra el servicio real con credenciales reales |
| Leer la hoja de estilos | MEDIR en el navegador: `getBoundingClientRect`, `scrollTop`, `elementsFromPoint`, la pestaña de red |
| Asumir el esquema | Describir la tabla con las herramientas de base de datos antes de escribir SQL |
| Suponer que el archivo llegó | Confirmar la petición en la red, no un `check()` que devuelve booleanos optimistas |
| Decir "debería funcionar" | Provocar el caso: crear el dato, cambiarlo, borrarlo, y volver a consultarlo |

## Cuando un arreglo corrige un PATRÓN

Buscar TODAS sus apariciones antes de cerrar: `rg "<llamada sospechosa>"` sobre el módulo, y
clasificar cada uso como correcto o defectuoso.

Un mismo defecto —pasar un token opaco a un parser de JWT— reapareció TRES veces porque el primer
arreglo tocó sólo el lugar donde se reportó.

## Antes de decir que está listo

- [ ] Se ejecutó contra el sistema real, no sólo contra dobles
- [ ] El caso contrario también se probó (no sólo el camino feliz)
- [ ] Si se afirmó algo visual, hay una MEDICIÓN que lo respalda
- [ ] Los datos de prueba se limpiaron, verificado con una consulta posterior
- [ ] Si se modificó un dato real por accidente, se restauró o se dijo explícitamente

## Cómo reportarlo

Dar el número, no el adjetivo. "199 → 319 px" comunica; "ahora se ve bien" no se puede verificar
ni contradecir.

Si algo quedó sin probar, decirlo en la misma frase en que se reporta lo demás.
