# FAQ Dockan

**Q : Dockan remplace-t-il Docker ?**
- Non, Dockan vise la simplicité, l’auto-hébergement, l’éducation, pas l’industrie ni le cloud.

**Q : Peut-on utiliser Dockan sans root ?**
- Oui, avec firejail. Certaines fonctions (chroot, mount) peuvent nécessiter sudo.

**Q : Où sont stockées les images ?**
- Chaque image est un dossier `.dockan/` ou une archive `.tar.gz`.

**Q : Peut-on partager une image Dockan ?**
- Oui, il suffit de partager le dossier ou l’archive (pas de cloud imposé).

**Q : Peut-on faire du réseau, des ports, etc. ?**
- Oui, mais Dockan ne gère pas le mapping de ports automatiquement. À gérer dans `start.sh`.

**Q : Peut-on utiliser Dockan sur un VPS, un Raspberry Pi ?**
- Oui, partout où Go et un outil d’isolation sont disponibles.

**Q : Comment contribuer ?**
- Forkez, proposez vos idées, partagez vos images Dockan !

Pour toute question, ouvrez une issue ou contactez la communauté.
