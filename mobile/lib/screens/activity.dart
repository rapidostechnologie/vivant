import 'package:flutter/material.dart';

import '../app_state.dart';
import '../theme.dart';
import '../widgets/page.dart';

/// Historique : connexions confirmées ou refusées.
class ActivityScreen extends StatefulWidget {
  const ActivityScreen({super.key});

  @override
  State<ActivityScreen> createState() => _ActivityScreenState();
}

class _ActivityScreenState extends State<ActivityScreen> {
  late final _me = AppScope.read(context).api.me(AppScope.read(context).identity!);

  static const _icons = {
    'auth_approved': (Icons.check_rounded, VColors.greenText),
    'auth_denied': (Icons.close_rounded, VColors.danger),
    'auth_binding_mismatch': (Icons.warning_amber_rounded, VColors.danger),
    'enrolled': (Icons.verified_rounded, VColors.greenText),
  };

  @override
  Widget build(BuildContext context) {
    return FutureBuilder(
      future: _me,
      builder: (context, snap) {
        final events = [
          for (final e in (snap.data?['history'] as List? ?? const []))
            if (_icons.containsKey(e['type'])) e as Map<String, dynamic>,
        ];
        return SubPage(
          title: 'Activité',
          children: [
            if (snap.connectionState != ConnectionState.done)
              const Center(child: Padding(padding: EdgeInsets.all(40), child: CircularProgressIndicator()))
            else if (snap.hasError)
              Text(snap.error.toString(), style: VText.subtitle)
            else if (events.isEmpty)
              const Text('Rien pour le moment.', style: VText.subtitle)
            else
              RowGroup(children: [
                for (final e in events)
                  ListTile(
                    minTileHeight: 64,
                    leading: Icon(_icons[e['type']]!.$1, color: _icons[e['type']]!.$2),
                    title: Text(e['detail'] as String? ?? '', style: const TextStyle(fontWeight: FontWeight.w600)),
                    subtitle: Text(_when(DateTime.parse(e['at'] as String).toLocal()),
                        style: const TextStyle(color: VColors.muted)),
                  ),
              ]),
          ],
        );
      },
    );
  }

  static String _when(DateTime t) {
    final now = DateTime.now();
    final hm = '${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}';
    if (now.year == t.year && now.month == t.month && now.day == t.day) return "Aujourd'hui, $hm";
    return '${t.day.toString().padLeft(2, '0')}/${t.month.toString().padLeft(2, '0')}/${t.year}, $hm';
  }
}
