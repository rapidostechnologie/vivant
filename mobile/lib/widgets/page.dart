import 'package:flutter/material.dart';

import '../theme.dart';

/// Page secondaire : titre en grand et contenu sur fond clair.
class SubPage extends StatelessWidget {
  const SubPage({super.key, required this.title, required this.children, this.back = true});

  final String title;
  final List<Widget> children;
  final bool back;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: VColors.background,
      appBar: back
          ? AppBar(
              backgroundColor: VColors.background,
              surfaceTintColor: Colors.transparent,
              leading: IconButton(
                tooltip: 'Retour',
                onPressed: () => Navigator.of(context).maybePop(),
                icon: const Icon(Icons.arrow_back_ios_new_rounded, size: 22),
              ),
            )
          : null,
      body: SafeArea(
        top: !back,
        child: ListView(
          padding: EdgeInsets.fromLTRB(20, back ? 0 : 24, 20, 32),
          children: [
            Text(title, style: VText.title.copyWith(fontSize: 32)),
            const SizedBox(height: 24),
            ...children,
          ],
        ),
      ),
    );
  }
}

/// Groupe de lignes sur une carte blanche.
class RowGroup extends StatelessWidget {
  const RowGroup({super.key, required this.children});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      decoration: BoxDecoration(color: Colors.white, borderRadius: BorderRadius.circular(22)),
      clipBehavior: Clip.antiAlias,
      child: Column(
        children: [
          for (var i = 0; i < children.length; i++) ...[
            if (i > 0) const Divider(height: 1, indent: 64, color: VColors.line),
            children[i],
          ],
        ],
      ),
    );
  }
}

class RowItem extends StatelessWidget {
  const RowItem({super.key, required this.icon, required this.label, this.value, this.onTap, this.danger = false});

  final IconData icon;
  final String label;
  final String? value;
  final VoidCallback? onTap;
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final color = danger ? VColors.danger : VColors.ink;
    return ListTile(
      onTap: onTap,
      minTileHeight: 60,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16),
      leading: Container(
        width: 36,
        height: 36,
        decoration: BoxDecoration(
          color: danger ? VColors.danger.withValues(alpha: 0.08) : VColors.soft,
          borderRadius: BorderRadius.circular(11),
        ),
        child: Icon(icon, size: 20, color: color),
      ),
      title: Text(label, style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600, color: color)),
      subtitle: value == null ? null : Text(value!, style: const TextStyle(color: VColors.muted)),
      trailing: onTap == null ? null : const Icon(Icons.chevron_right_rounded, color: VColors.muted),
    );
  }
}
