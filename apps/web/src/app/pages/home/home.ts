import { ChangeDetectionStrategy, Component } from '@angular/core';

@Component({
  selector: 'pixela-home',
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './home.html',
  styleUrl: './home.scss',
})
export class Home {}
