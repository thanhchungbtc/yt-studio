import type { Chapter, Task, Video } from '../../../core/types'

/**
 * What every mode is handed, and the whole of what it is handed.
 *
 * One shape for all of them so the mode list can be a table rather than a
 * branch: the editor fetches the video and its two collections once, and each
 * view reads the parts it needs and ignores the rest. A mode that wanted a
 * fourth thing would fetch it itself — react-query is already holding these,
 * so a view asking for one of them again costs a cache read.
 */
export interface ViewProps {
  video: Video
  chapters: Chapter[]
  tasks: Task[]
}
