import { AL_MangaDetailsById_Media_Rankings, AL_MediaDetailsById_Media_Rankings } from "@/api/generated/types"
import { useServerStatus } from "@/app/(main)/_hooks/server-status.hooks"
import { Badge } from "@/components/ui/badge"
import { IconButton } from "@/components/ui/button"
import { Disclosure, DisclosureContent, DisclosureItem, DisclosureTrigger } from "@/components/ui/disclosure"
import { Tooltip } from "@/components/ui/tooltip"
import capitalize from "lodash/capitalize"
import React, { useMemo } from "react"
import { AiFillStar, AiOutlineHeart, AiOutlineStar } from "react-icons/ai"
import { BiHeart, BiHide } from "react-icons/bi"

type MediaEntryGenresListProps = {
    genres?: Array<string | null> | null | undefined
}

export function MediaEntryGenresList(props: MediaEntryGenresListProps) {

    const {
        genres,
        ...rest
    } = props

    if (!genres) return null
    return (
        <>
            <div className="items-center flex flex-wrap gap-2">
                {genres?.map(genre => {
                    return <Badge key={genre!} className="border-transparent" size="lg">{genre}</Badge>
                })}
            </div>
        </>
    )
}

type MediaEntryAudienceScoreProps = {
    meanScore?: number | null
}

export function MediaEntryAudienceScore(props: MediaEntryAudienceScoreProps) {

    const {
        meanScore,
        ...rest
    } = props

    const status = useServerStatus()
    const hideAudienceScore = useMemo(() => status?.settings?.anilist?.hideAudienceScore ?? false, [status?.settings?.anilist?.hideAudienceScore])

    if (!meanScore) return null

    const ScoreBadge = (
        <Badge
            className=""
            size="lg"
            intent={meanScore >= 70 ? meanScore >= 82 ? "primary" : "success" : "gray"}
            leftIcon={<BiHeart />}
        >{meanScore / 10}</Badge>
    )

    return (
        <>
            {hideAudienceScore ? <Disclosure type="single" collapsible>
                <DisclosureItem value="item-1" className="flex items-center gap-1">
                    <Tooltip
                        side="right"
                        trigger={<DisclosureTrigger>
                            <IconButton
                                intent="gray-basic"
                                icon={<BiHide className="text-sm" />}
                                rounded
                                size="sm"
                            />
                        </DisclosureTrigger>}
                    >Show audience score</Tooltip>
                    <DisclosureContent>
                        {ScoreBadge}
                    </DisclosureContent>
                </DisclosureItem>
            </Disclosure> : ScoreBadge}
        </>
    )
}

type AnimeEntryStudioProps = {
    studios?: { nodes?: Array<{ name: string } | null> | null } | null | undefined
}

export function AnimeEntryStudio(props: AnimeEntryStudioProps) {

    const {
        studios,
        ...rest
    } = props

    if (!studios?.nodes) return null

    return (
        <>
            <Badge
                size="lg"
                intent="gray"
                className="rounded-full border-transparent"
            >
                {studios?.nodes?.[0]?.name}
            </Badge>
        </>
    )
}

type MediaEntryRankingsProps = {
    rankings?: AL_MediaDetailsById_Media_Rankings[] | AL_MangaDetailsById_Media_Rankings[]
}

export function MediaEntryRankings(props: MediaEntryRankingsProps) {

    const {
        rankings,
        ...rest
    } = props

    const seasonMostPopular = rankings?.find(r => (!!r?.season || !!r?.year) && r?.type === "POPULAR" && r.rank <= 10)
    const allTimeHighestRated = rankings?.find(r => !!r?.allTime && r?.type === "RATED" && r.rank <= 100)
    const seasonHighestRated = rankings?.find(r => (!!r?.season || !!r?.year) && r?.type === "RATED" && r.rank <= 5)
    const allTimeMostPopular = rankings?.find(r => !!r?.allTime && r?.type === "POPULAR" && r.rank <= 100)

    const formatFormat = React.useCallback((format: string) => {
        return (format === "TV" ? "" : format).replace("_", " ")
    }, [])

    if (!rankings) return null

    return (
        <>
            {(!!allTimeHighestRated || !!seasonMostPopular) && <div className="flex-wrap gap-2 hidden md:flex">
                {allTimeHighestRated && <Badge
                    size="lg"
                    intent="gray"
                    leftIcon={<AiFillStar />}
                    iconClass="text-yellow-500"
                    className="rounded-md border-transparent px-2"
                >
                    #{String(allTimeHighestRated.rank)} Highest
                    Rated {formatFormat(allTimeHighestRated.format)} of All
                    Time
                </Badge>}
                {seasonHighestRated && <Badge
                    size="lg"
                    intent="gray"
                    leftIcon={<AiOutlineStar />}
                    iconClass="text-yellow-500"
                    className="rounded-md border-transparent px-2"
                >
                    #{String(seasonHighestRated.rank)} Highest
                    Rated {formatFormat(seasonHighestRated.format)} of {capitalize(seasonHighestRated.season!)} {seasonHighestRated.year}
                </Badge>}
                {seasonMostPopular && <Badge
                    size="lg"
                    intent="gray"
                    leftIcon={<AiOutlineHeart />}
                    iconClass="text-pink-500"
                    className="rounded-md border-transparent px-2"
                >
                    #{(String(seasonMostPopular.rank))} Most
                    Popular {formatFormat(seasonMostPopular.format)} of {capitalize(seasonMostPopular.season!)} {seasonMostPopular.year}
                </Badge>}
            </div>}
        </>
    )
}
